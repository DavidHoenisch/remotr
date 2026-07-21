package resourceregistry

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secretref"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"gopkg.in/yaml.v3"
)

// EffectiveHash derives the shared canonical hash for this normalized typed
// resource and its selected provider. Secret identities must already be
// resolved to safe provider/version metadata.
func (r Resource) EffectiveHash(address, providerID string, secrets []effectivehash.SecretIdentity) (string, error) {
	desiredNode, err := r.desiredHashNode()
	if err != nil {
		return "", err
	}
	desiredNode, desiredReferences, err := r.secretSafeHashNode(desiredNode)
	if err != nil {
		return "", fmt.Errorf("resource %q desired hash input: %w", address, err)
	}
	desired, err := yamlObject(desiredNode)
	if err != nil {
		return "", fmt.Errorf("resource %q desired hash input: %w", address, err)
	}
	defaultNode, err := typedHashNode(r.value)
	if err != nil {
		return "", fmt.Errorf("resource %q normalized hash input: %w", address, err)
	}
	defaultNode, defaultReferences, err := r.secretSafeHashNode(defaultNode)
	if err != nil {
		return "", fmt.Errorf("resource %q normalized hash input: %w", address, err)
	}
	defaults, err := yamlObject(defaultNode)
	if err != nil {
		return "", fmt.Errorf("resource %q normalized hash input: %w", address, err)
	}
	references := mergeHashReferences(desiredReferences, defaultReferences)
	if err := validateSecretIdentities(references, secrets); err != nil {
		return "", fmt.Errorf("resource %q secret identity: %w", address, err)
	}
	return effectivehash.Sum(effectivehash.Input{
		ResourceAddress: address,
		ResourceKind:    string(r.Kind()),
		Provider: effectivehash.ProviderIdentity{
			ID: providerID, ContractRevision: r.ProviderContractRevision(),
		},
		Desired:  desired,
		Defaults: defaults,
		Secrets:  append([]effectivehash.SecretIdentity(nil), secrets...),
	})
}

// ResolveEffectiveHash resolves every declared secret reference to safe
// identity metadata, clears returned material, and derives the canonical hash.
func (r Resource) ResolveEffectiveHash(ctx context.Context, address, providerID, artifactDigest string, resolver secrets.Resolver) (string, error) {
	desiredNode, err := r.desiredHashNode()
	if err != nil {
		return "", err
	}
	_, references, err := r.secretSafeHashNode(desiredNode)
	if err != nil {
		return "", err
	}
	if len(references) == 0 {
		return r.EffectiveHash(address, providerID, nil)
	}
	if resolver == nil {
		return "", fmt.Errorf("resource %q requires a secret identity resolver", address)
	}
	identities := make([]effectivehash.SecretIdentity, 0, len(references))
	for _, reference := range references {
		purpose, err := secretHashPurpose(r.Kind(), reference.path)
		if err != nil {
			return "", err
		}
		resolved, err := resolver.Resolve(ctx, secrets.ResolveRequest{
			Reference: reference.reference.String(), ArtifactDigest: artifactDigest,
			ResourceAddress: address, Purpose: purpose,
		})
		if err != nil {
			return "", secrets.RedactedResolutionError(err)
		}
		identity := effectivehash.SecretIdentity{
			Path: reference.path, Provider: resolved.Provider, Name: reference.reference.Name,
			Version: resolved.Version, ActivationGeneration: resolved.ActivationGeneration, Purpose: purpose,
		}
		clear(resolved.Material)
		if identity.Provider != reference.reference.Provider {
			return "", fmt.Errorf("secret provider identity mismatch for %q", reference.path)
		}
		if identity.Provider == secretref.ProviderLocalFile && identity.Version == "" {
			identity.Version = "external"
		}
		identities = append(identities, identity)
	}
	return r.EffectiveHash(address, providerID, identities)
}

func secretHashPurpose(kind models.ResourceKind, path string) (string, error) {
	purposes := map[models.ResourceKind]map[string]string{
		models.ResourceKindAPTRepository:    {"credentialRef": "repository-credential"},
		models.ResourceKindPacmanRepository: {"credentialRef": "repository-credential"},
		models.ResourceKindDownload:         {"authenticationRef": "download-authentication"},
		models.ResourceKindUser:             {"passwordHashRef": "password-hash"},
		models.ResourceKindNetworkProfile:   {"credentialRef": "network-credential"},
		models.ResourceKindCertificate:      {"certificateRef": "certificate-public", "chainRefs[]": "certificate-chain", "privateKeyRef": "certificate-private-key"},
		models.ResourceKindTrustAnchor:      {"anchorRef": "ca-trust-anchor"},
		models.ResourceKindEndpointSchedule: {"environment[].secretRef": "schedule-environment"},
		models.ResourceKindAgentInstall:     {"enrollmentTokenSecret": "agent-enrollment-token"},
		models.ResourceKindUbuntuPro: {
			"tokenRef":                     "ubuntu-pro-token",
			"landscape.registrationKeyRef": "landscape-registration-key",
			"landscape.caRef":              "landscape-ca",
		},
	}
	if purpose := purposes[kind][path]; purpose != "" {
		return purpose, nil
	}
	return "", fmt.Errorf("resource kind %q secret field %q has no hash purpose", kind, path)
}

type hashReference struct {
	path      string
	reference secretref.Reference
}

func (r Resource) secretSafeHashNode(node *yaml.Node) (*yaml.Node, []hashReference, error) {
	projected, keep, references, err := stripSecretHashValues(node, "", r.definition.FieldDescriptors)
	if err != nil {
		return nil, nil, err
	}
	if !keep || projected == nil || projected.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("resource root was removed by secret projection")
	}
	return projected, references, nil
}

func stripSecretHashValues(node *yaml.Node, path string, descriptors FieldDescriptors) (*yaml.Node, bool, []hashReference, error) {
	if node == nil {
		return nil, false, nil, fmt.Errorf("YAML value is required")
	}
	if node.Kind == yaml.AliasNode {
		return stripSecretHashValues(node.Alias, path, descriptors)
	}
	if descriptor, descriptorPath, ok := hashDescriptor(path, descriptors); ok && descriptor.Sensitivity == SensitivitySecret && node.Kind == yaml.ScalarNode {
		if descriptor.Projection != ProjectReference {
			return nil, false, nil, nil
		}
		reference, err := secretref.ParseSelected(node.Value)
		if err != nil {
			return nil, false, nil, fmt.Errorf("secret reference field %q: %w", descriptorPath, err)
		}
		return nil, false, []hashReference{{path: descriptorPath, reference: reference}}, nil
	}
	clone := *node
	clone.Alias = nil
	clone.Content = nil
	var allReferences []hashReference
	switch node.Kind {
	case yaml.MappingNode:
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index].Value
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			child, keep, references, err := stripSecretHashValues(node.Content[index+1], childPath, descriptors)
			if err != nil {
				return nil, false, nil, err
			}
			if !keep {
				allReferences = append(allReferences, references...)
				continue
			}
			clone.Content = append(clone.Content, cloneYAMLNode(node.Content[index]), child)
			allReferences = append(allReferences, references...)
		}
		return &clone, true, allReferences, nil
	case yaml.SequenceNode:
		childPath := path + "[]"
		for _, source := range node.Content {
			child, keep, references, err := stripSecretHashValues(source, childPath, descriptors)
			if err != nil {
				return nil, false, nil, err
			}
			allReferences = append(allReferences, references...)
			if keep {
				clone.Content = append(clone.Content, child)
			}
		}
		if len(node.Content) > 0 && len(clone.Content) == 0 {
			return nil, false, allReferences, nil
		}
		return &clone, true, allReferences, nil
	default:
		return &clone, true, nil, nil
	}
}

func hashDescriptor(path string, descriptors FieldDescriptors) (FieldDescriptor, string, bool) {
	if descriptor, ok := descriptors[path]; ok {
		return descriptor, path, true
	}
	for pattern, descriptor := range descriptors {
		if hashPathMatches(pattern, path) {
			return descriptor, pattern, true
		}
	}
	return FieldDescriptor{}, "", false
}

func hashPathMatches(pattern, actual string) bool {
	patternParts := strings.Split(pattern, ".")
	actualParts := strings.Split(actual, ".")
	if len(patternParts) != len(actualParts) {
		return false
	}
	for index := range patternParts {
		if patternParts[index] != "*" && patternParts[index] != actualParts[index] {
			return false
		}
	}
	return true
}

func mergeHashReferences(groups ...[]hashReference) []hashReference {
	seen := make(map[string]struct{})
	var merged []hashReference
	for _, group := range groups {
		for _, reference := range group {
			key := reference.path + "\x00" + reference.reference.String()
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, reference)
		}
	}
	return merged
}

func validateSecretIdentities(references []hashReference, identities []effectivehash.SecretIdentity) error {
	used := make([]bool, len(identities))
	for _, required := range references {
		matched := false
		for index, identity := range identities {
			if used[index] || identity.Path != required.path || identity.Provider != required.reference.Provider || identity.Name != required.reference.Name {
				continue
			}
			if !required.reference.FollowsActive() && identity.Version != required.reference.Selector {
				continue
			}
			if required.reference.FollowsActive() && identity.ActivationGeneration == 0 {
				continue
			}
			used[index] = true
			matched = true
			break
		}
		if !matched {
			return fmt.Errorf("resolved identity for %s %q is required", required.path, required.reference.Name)
		}
	}
	for index, identityUsed := range used {
		if !identityUsed {
			return fmt.Errorf("unexpected resolved identity for %s %q", identities[index].Path, identities[index].Name)
		}
	}
	return nil
}

func (r Resource) desiredHashNode() (*yaml.Node, error) {
	if r.source != nil {
		return removeMappingKey(r.source, "kind")
	}
	return typedHashNode(r.value)
}

func typedHashNode(value any) (*yaml.Node, error) {
	raw, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("typed resource is not a mapping")
	}
	return removeMappingKey(document.Content[0], "kind")
}

func yamlObject(node *yaml.Node) (effectivehash.Object, error) {
	value, err := yamlValue(node)
	if err != nil {
		return nil, err
	}
	object, ok := value.(effectivehash.Object)
	if !ok {
		return nil, fmt.Errorf("resource root must be an object")
	}
	return object, nil
}

func yamlValue(node *yaml.Node) (effectivehash.Value, error) {
	if node == nil {
		return nil, fmt.Errorf("YAML value is required")
	}
	if node.Kind == yaml.AliasNode {
		return yamlValue(node.Alias)
	}
	switch node.Kind {
	case yaml.MappingNode:
		object := make(effectivehash.Object, len(node.Content)/2)
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index].Value
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate mapping key %q", key)
			}
			value, err := yamlValue(node.Content[index+1])
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", key, err)
			}
			object[key] = value
		}
		return object, nil
	case yaml.SequenceNode:
		list := make(effectivehash.List, len(node.Content))
		for index, child := range node.Content {
			value, err := yamlValue(child)
			if err != nil {
				return nil, fmt.Errorf("list item %d: %w", index, err)
			}
			list[index] = value
		}
		return list, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!null":
			return effectivehash.Null{}, nil
		case "!!bool":
			value, err := strconv.ParseBool(node.Value)
			if err != nil {
				return nil, err
			}
			return effectivehash.Boolean(value), nil
		case "!!int":
			if strings.HasPrefix(node.Value, "-") {
				value, err := strconv.ParseInt(node.Value, 0, 64)
				if err != nil {
					return nil, err
				}
				return effectivehash.Integer(value), nil
			}
			if value, err := strconv.ParseUint(node.Value, 0, 64); err == nil {
				return effectivehash.Unsigned(value), nil
			}
			value, err := strconv.ParseInt(node.Value, 0, 64)
			if err != nil {
				return nil, err
			}
			return effectivehash.Integer(value), nil
		case "!!float":
			value, err := strconv.ParseFloat(node.Value, 64)
			if err != nil {
				return nil, err
			}
			return effectivehash.Float(value), nil
		default:
			return effectivehash.String(node.Value), nil
		}
	default:
		return nil, fmt.Errorf("unsupported YAML node kind %d", node.Kind)
	}
}

func defaultProviderContractRevision(kind models.ResourceKind) string {
	if kind == models.ResourceKindService {
		return "service-state-v1"
	}
	if kind == models.ResourceKindUbuntuPro {
		return "ubuntu-pro-v1"
	}
	return string(kind) + "-v1"
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	clone := *node
	clone.Content = make([]*yaml.Node, len(node.Content))
	for index, child := range node.Content {
		clone.Content[index] = cloneYAMLNode(child)
	}
	clone.Alias = cloneYAMLNode(node.Alias)
	return &clone
}
