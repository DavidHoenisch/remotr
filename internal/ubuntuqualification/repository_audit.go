package ubuntuqualification

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/traceability"
)

// LoadRepositoryAudit derives the Ubuntu closeout decision from the canonical
// checked-in manifest, provider matrix, traceability map, and sibling changes.
func LoadRepositoryAudit(repositoryRoot string) (AuditReport, error) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return AuditReport{}, err
	}
	manifest, err := Load(filepath.Join(repositoryRoot, "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		return AuditReport{}, fmt.Errorf("load qualification manifest: %w", err)
	}
	matrix, err := providermatrix.Load(filepath.Join(repositoryRoot, "test", "provider-matrix.yaml"))
	if err != nil {
		return AuditReport{}, fmt.Errorf("load provider matrix: %w", err)
	}
	trace, err := traceability.LoadManifest(filepath.Join(repositoryRoot, "test", "traceability.yaml"))
	if err != nil {
		return AuditReport{}, fmt.Errorf("load traceability manifest: %w", err)
	}
	dependencies, err := loadRepositoryDependencies(repositoryRoot, matrix.Dependencies)
	if err != nil {
		return AuditReport{}, err
	}
	return generateRepositoryAudit(manifest, matrix, trace, dependencies)
}

func generateRepositoryAudit(manifest Manifest, matrix providermatrix.Matrix, trace traceability.Manifest, dependencies []DependencyGate) (AuditReport, error) {
	targets := make([]AuditTarget, 0, len(manifest.Rows))
	for _, row := range manifest.Rows {
		milestone, err := milestoneForCapability(row.CapabilityID)
		if err != nil {
			return AuditReport{}, err
		}
		target := AuditTarget{
			Milestone: milestone,
			RowKey:    exactKey(row.CapabilityID, row.Backend, row.ContractRevision, row.Environment),
			Selector:  row.Selectors[0],
		}
		matches := matchingMatrixRows(row, matrix.Rows)

		switch row.Disposition {
		case "unadvertised":
			target.Status = "unadvertised"
			target.ExplicitlyDescoped = true
			for _, evidence := range matches {
				if evidence.Status == "passing" {
					target.Status = "blocked"
					target.ExplicitlyDescoped = false
					target.Selector = evidence.Selectors[0]
					break
				}
			}
		case "blocked":
			target.Status = "blocked"
		case "qualified":
			target.Status, target.Selector = qualifiedRepositoryStatus(row, matches, trace)
		default:
			return AuditReport{}, fmt.Errorf("audit row %s has unsupported disposition %q", target.RowKey, row.Disposition)
		}
		targets = append(targets, target)
	}
	return GenerateAudit(targets, dependencies)
}

func qualifiedRepositoryStatus(row Row, matches []providermatrix.Row, trace traceability.Manifest) (string, string) {
	selector := row.Selectors[0]
	if len(matches) == 0 {
		return "missing", selector
	}
	if len(matches) != 1 {
		return "blocked", selector
	}
	evidence := matches[0]
	if len(evidence.Selectors) > 0 {
		selector = evidence.Selectors[0]
	}
	if evidence.Status != "passing" {
		return evidence.Status, selector
	}
	for _, evidenceSelector := range evidence.Selectors {
		if !slices.Contains(row.Selectors, evidenceSelector) {
			return "blocked", evidenceSelector
		}
	}
	for _, id := range row.GoverningIDs {
		entry, ok := trace.Scenarios[id]
		if !ok {
			return "missing", "traceability:" + id
		}
		if entry.Lifecycle != "verified" {
			if len(entry.Selectors) > 0 {
				selector = entry.Selectors[0]
			} else {
				selector = "traceability:" + id
			}
			if entry.Lifecycle == "planned" {
				return "planned", selector
			}
			return "blocked", selector
		}
	}
	return "qualified", selector
}

func matchingMatrixRows(row Row, rows []providermatrix.Row) []providermatrix.Row {
	environment := row.Environment
	if strings.HasPrefix(environment, "vm-") {
		environment = "vm"
	}
	var matches []providermatrix.Row
	for _, evidence := range rows {
		if evidence.CapabilityID == row.CapabilityID && evidence.Backend == row.Backend &&
			evidence.ContractRevision == row.ContractRevision && evidence.Distribution == row.Distribution &&
			evidence.Release == row.Release && evidence.Architecture == row.Architecture && evidence.Environment == environment {
			matches = append(matches, evidence)
		}
	}
	return matches
}

func milestoneForCapability(capabilityID string) (string, error) {
	switch capabilityID {
	case "file", "download":
		return "M1", nil
	case "directory", "link", "group", "user", "authorizedKey", "knownHost", "sudo", "userFile":
		return "M2", nil
	case "sysctl", "kernelModule", "hostname", "hostLocale", "timeSync", "mount", "swap":
		return "M3", nil
	case "endpointSchedule", "service", "systemd", "systemdUser", "systemdUnit", "reboot", "hostsEntry", "dnsResolver", "route", "networkProfile", "firewall":
		return "M4", nil
	case "certificate", "trustAnchor", "appArmorProfile", "auditRules", "accountLimit", "loginPolicy", "journald", "logrotate", "desktopSetting", "sessionPolicy", "browserPolicy":
		return "M5", nil
	case "bootstrap", "agentInstall", "command":
		return "", nil
	default:
		return "", fmt.Errorf("qualification audit has no milestone mapping for capability %q", capabilityID)
	}
}

func loadRepositoryDependencies(repositoryRoot string, matrix providermatrix.DependencyGates) ([]DependencyGate, error) {
	types := []struct {
		name           string
		matrixAccepted bool
	}{
		{"complete-applicator-execution-contract", matrix.ExecutionContract},
		{"complete-capability-compatible-delivery", matrix.CapabilityDelivery},
		{"complete-core-package-providers", matrix.PackageProviders},
		{"establish-testing-and-performance-foundation", matrix.TestingFoundation},
	}
	dependencies := make([]DependencyGate, 0, len(types))
	for _, dependency := range types {
		tasksPath, err := dependencyTasksPath(repositoryRoot, dependency.name)
		if err != nil {
			return nil, fmt.Errorf("audit dependency %s: %w", dependency.name, err)
		}
		checked, err := changeTasksComplete(tasksPath)
		if err != nil {
			return nil, fmt.Errorf("audit dependency %s: %w", dependency.name, err)
		}
		gate := DependencyGate{Name: dependency.name, Accepted: dependency.matrixAccepted && checked}
		switch {
		case !dependency.matrixAccepted:
			gate.Reason = "provider matrix dependency gate is not accepted"
		case !checked:
			gate.Reason = "OpenSpec task checklist is incomplete"
		}
		dependencies = append(dependencies, gate)
	}
	return dependencies, nil
}

func dependencyTasksPath(repositoryRoot, changeName string) (string, error) {
	activeChangePath := filepath.Join(repositoryRoot, "openspec", "changes", changeName)
	activeTasksPath := filepath.Join(activeChangePath, "tasks.md")
	if _, err := os.Stat(activeChangePath); err == nil {
		if _, err := os.Stat(activeTasksPath); err == nil {
			return activeTasksPath, nil
		} else if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("active change task checklist not found")
		} else {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	archivePattern := filepath.Join(repositoryRoot, "openspec", "changes", "archive", "*-"+changeName, "tasks.md")
	matches, err := filepath.Glob(archivePattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("task checklist not found in active changes or archive")
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple archived task checklists found")
	}
	return matches[0], nil
}

func changeTasksComplete(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	foundTask := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "- [") {
			foundTask = true
		}
		if strings.HasPrefix(line, "- [ ]") {
			return false, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	if !foundTask {
		return false, fmt.Errorf("task checklist has no tasks")
	}
	return true, nil
}
