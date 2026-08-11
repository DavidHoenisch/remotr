package secrets

import (
	"context"
	"errors"
	"sync"

	"github.com/DavidHoenisch/remotr/internal/performance"
)

const (
	defaultAuthorityCacheEntries = 128
	defaultAuthorityCacheBytes   = 8 << 20
)

// AuthorityCacheOptions bounds server-managed plaintext retained by an agent.
type AuthorityCacheOptions struct {
	MaxEntries       int
	MaxMaterialBytes int
}

type authorityCacheKey struct {
	reference       string
	endpointID      string
	fleet           string
	artifactDigest  string
	resourceAddress string
	purpose         string
}

type authorityCacheEntry struct {
	resolved     Resolved
	unauthorized bool
	lastUsed     uint64
}

// AuthorityCachingResolver reuses server-managed results only while a Sync
// authority token proves that the endpoint's resolution authority is stable.
type AuthorityCachingResolver struct {
	mu               sync.Mutex
	delegate         Resolver
	maxEntries       int
	maxMaterialBytes int
	materialBytes    int
	sequence         uint64
	token            string
	entries          map[authorityCacheKey]authorityCacheEntry
}

// NewAuthorityCachingResolver wraps the authenticated Remotr provider.
func NewAuthorityCachingResolver(
	delegate Resolver,
	options AuthorityCacheOptions,
) *AuthorityCachingResolver {
	if options.MaxEntries <= 0 {
		options.MaxEntries = defaultAuthorityCacheEntries
	}
	if options.MaxMaterialBytes <= 0 {
		options.MaxMaterialBytes = defaultAuthorityCacheBytes
	}
	return &AuthorityCachingResolver{
		delegate:         delegate,
		maxEntries:       options.MaxEntries,
		maxMaterialBytes: options.MaxMaterialBytes,
		entries:          make(map[authorityCacheKey]authorityCacheEntry),
	}
}

// SetAuthorityToken observes the latest authenticated Sync authority. A
// changed or missing token clears all retained results before later use.
func (r *AuthorityCachingResolver) SetAuthorityToken(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if token == r.token {
		return
	}
	if r.token != "" {
		performance.RecordSecretAuthorityInvalidation()
	}
	if token == "" {
		performance.RecordSecretAuthorityFailClosed()
	}
	r.clearLocked()
	r.token = token
}

// Resolve returns a copy of a stable cached result or calls the delegate.
func (r *AuthorityCachingResolver) Resolve(
	ctx context.Context,
	request ResolveRequest,
) (Resolved, error) {
	provider, _, parseErr := ParseReference(request.Reference)
	if parseErr != nil || provider != ProviderRemotr {
		return r.delegate.Resolve(ctx, request)
	}
	key := authorityKey(request)
	for {
		r.mu.Lock()
		token := r.token
		if token != "" {
			if entry, ok := r.entries[key]; ok {
				r.sequence++
				entry.lastUsed = r.sequence
				r.entries[key] = entry
				r.mu.Unlock()
				if entry.unauthorized {
					performance.RecordSecretAuthorityHit(true)
					return Resolved{}, ErrUnauthorized
				}
				performance.RecordSecretAuthorityHit(false)
				return cloneResolved(entry.resolved), nil
			}
		}
		r.mu.Unlock()
		if token == "" {
			performance.RecordSecretAuthorityFailClosed()
		}

		resolved, err := r.delegate.Resolve(ctx, request)
		if token == "" {
			return resolved, err
		}

		r.mu.Lock()
		if r.token != token {
			r.mu.Unlock()
			clear(resolved.Material)
			continue
		}
		if errors.Is(err, ErrUnauthorized) {
			r.insertLocked(key, authorityCacheEntry{unauthorized: true})
			performance.RecordSecretAuthorityPrime()
			r.mu.Unlock()
			return resolved, err
		}
		if err != nil || len(resolved.Material) == 0 ||
			len(resolved.Material) > r.maxMaterialBytes {
			if err == nil && len(resolved.Material) > r.maxMaterialBytes {
				performance.RecordSecretAuthorityDeclined()
			}
			r.mu.Unlock()
			return resolved, err
		}
		entry := authorityCacheEntry{resolved: cloneResolved(resolved)}
		r.insertLocked(key, entry)
		performance.RecordSecretAuthorityPrime()
		r.mu.Unlock()
		return resolved, nil
	}
}

func authorityKey(request ResolveRequest) authorityCacheKey {
	return authorityCacheKey{
		reference:       request.Reference,
		endpointID:      request.EndpointID,
		fleet:           request.Fleet,
		artifactDigest:  request.ArtifactDigest,
		resourceAddress: request.ResourceAddress,
		purpose:         request.Purpose,
	}
}

func cloneResolved(input Resolved) Resolved {
	output := input
	output.Material = append([]byte(nil), input.Material...)
	return output
}

func (r *AuthorityCachingResolver) insertLocked(
	key authorityCacheKey,
	entry authorityCacheEntry,
) {
	if existing, ok := r.entries[key]; ok {
		r.removeLocked(key, existing)
	}
	for len(r.entries) >= r.maxEntries ||
		r.materialBytes+len(entry.resolved.Material) > r.maxMaterialBytes {
		if !r.evictLocked() {
			return
		}
	}
	r.sequence++
	entry.lastUsed = r.sequence
	r.entries[key] = entry
	r.materialBytes += len(entry.resolved.Material)
}

func (r *AuthorityCachingResolver) evictLocked() bool {
	var victim authorityCacheKey
	var victimEntry authorityCacheEntry
	found := false
	for key, entry := range r.entries {
		if !found || entry.lastUsed < victimEntry.lastUsed ||
			entry.lastUsed == victimEntry.lastUsed && lessAuthorityKey(key, victim) {
			victim = key
			victimEntry = entry
			found = true
		}
	}
	if !found {
		performance.RecordSecretAuthorityDeclined()
		return false
	}
	r.removeLocked(victim, victimEntry)
	performance.RecordSecretAuthorityEviction()
	return true
}

func lessAuthorityKey(left, right authorityCacheKey) bool {
	return left.reference+"\x00"+left.endpointID+"\x00"+left.fleet+"\x00"+
		left.artifactDigest+"\x00"+left.resourceAddress+"\x00"+left.purpose <
		right.reference+"\x00"+right.endpointID+"\x00"+right.fleet+"\x00"+
			right.artifactDigest+"\x00"+right.resourceAddress+"\x00"+right.purpose
}

func (r *AuthorityCachingResolver) removeLocked(
	key authorityCacheKey,
	entry authorityCacheEntry,
) {
	clear(entry.resolved.Material)
	delete(r.entries, key)
	r.materialBytes -= len(entry.resolved.Material)
}

func (r *AuthorityCachingResolver) clearLocked() {
	for key, entry := range r.entries {
		clear(entry.resolved.Material)
		delete(r.entries, key)
	}
	r.materialBytes = 0
}

func (r *AuthorityCachingResolver) bounds() (entries, materialBytes int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries), r.materialBytes
}
