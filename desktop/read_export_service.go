package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/executor"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	readExportEndpointLimit     = 5000
	readExportConcurrencyLimit  = 10
	readExportRulesetLimit      = 1 << 20
	readExportZoneLimit         = 500
	readExportListLimit         = 500
	readExportSystemReportLimit = 2 << 20
	readExportTextLimit         = 2048
)

type ReadExportDialogRequest struct {
	Title         string
	SuggestedName string
	DisplayName   string
	Pattern       string
}

type ReadExportSaveDialog func(context.Context, ReadExportDialogRequest) (string, error)

type ReadExportService struct {
	chooseDestination ReadExportSaveDialog
	now               func() time.Time
}

func NewReadExportService(dialog ReadExportSaveDialog) *ReadExportService {
	return &ReadExportService{chooseDestination: dialog, now: time.Now}
}

func defaultReadExportSaveDialog(ctx context.Context, request ReadExportDialogRequest) (string, error) {
	return wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{
		Title:                request.Title,
		DefaultFilename:      request.SuggestedName,
		CanCreateDirectories: true,
		Filters: []wailsruntime.FileFilter{{
			DisplayName: request.DisplayName,
			Pattern:     request.Pattern,
		}},
	})
}

func (s *ReadExportService) LoadAssetInventoryConnected(ctx context.Context, client *admin.Client) (AssetInventoryView, error) {
	if client == nil {
		return AssetInventoryView{}, ErrSessionNotConnected
	}
	endpoints, err := client.ListEndpointsContext(ctx)
	if err != nil {
		return AssetInventoryView{}, err
	}
	if len(endpoints) > readExportEndpointLimit {
		return AssetInventoryView{}, &ClassifiedError{
			Kind:     ErrorValidation,
			Message:  "Asset inventory exceeds the supported Endpoint limit.",
			Guidance: "Narrow the managed scope before loading complete asset inventory.",
		}
	}

	rows := make([]AssetInventoryRow, len(endpoints))
	omitted := make([]string, 0)
	var mu sync.Mutex
	tasks := make([]func(context.Context), 0, len(endpoints))
	for index, endpoint := range endpoints {
		index, endpointID := index, endpoint.ID
		tasks = append(tasks, func(taskCtx context.Context) {
			detail, detailErr := client.GetEndpointContext(taskCtx, endpointID)
			mu.Lock()
			defer mu.Unlock()
			if detailErr != nil || detail.ID != endpointID {
				omitted = append(omitted, endpointID)
				return
			}
			rows[index] = mapAssetInventoryRow(detail)
		})
	}
	runWorkspaceTasks(ctx, readExportConcurrencyLimit, tasks)
	if cause := context.Cause(ctx); cause != nil {
		return AssetInventoryView{}, cause
	}
	filled := rows[:0]
	for _, row := range rows {
		if row.EndpointID != "" {
			filled = append(filled, row)
		}
	}
	slices.SortFunc(filled, func(left, right AssetInventoryRow) int {
		return strings.Compare(left.EndpointID, right.EndpointID)
	})
	slices.Sort(omitted)
	loadedAt := s.now().UTC()
	section := workspaceSectionResult("Asset inventory", nil, len(filled), loadedAt, latestEndpointObservation(endpoints))
	if len(omitted) > 0 {
		failedAt := formatTimestamp(loadedAt)
		section.State = SectionPartial
		section.Snapshot.FailedAt = &failedAt
		section.Error = &ClassifiedError{
			Kind:     ErrorUnexpected,
			Message:  "Some Endpoint asset records could not be loaded.",
			Guidance: "Refresh inventory before exporting or investigate the omitted Endpoint IDs.",
		}
	}
	return AssetInventoryView{Rows: filled, OmittedEndpointIDs: omitted, Section: section}, nil
}

func (s *ReadExportService) SaveAssetInventoryConnected(ctx context.Context, client *admin.Client, format string) (ReadExportSaveResult, error) {
	format, err := validateReadExportFormat(format)
	if err != nil {
		return ReadExportSaveResult{}, err
	}
	view, err := s.LoadAssetInventoryConnected(ctx, client)
	if err != nil {
		return ReadExportSaveResult{}, err
	}
	var data []byte
	if format == "json" {
		data, err = json.MarshalIndent(view.Rows, "", "  ")
		data = append(data, '\n')
	} else {
		data, err = encodeAssetInventoryCSV(view.Rows)
	}
	if err != nil {
		return ReadExportSaveResult{}, fmt.Errorf("encode asset inventory: %w", err)
	}
	return s.save(ctx, data, ReadExportDialogRequest{
		Title:         "Save asset inventory",
		SuggestedName: "remotr-inventory." + format,
		DisplayName:   strings.ToUpper(format) + " (*." + format + ")",
		Pattern:       "*." + format,
	})
}

func (s *ReadExportService) LoadFleetOperationalReportsConnected(ctx context.Context, client *admin.Client, fleet string) (FleetOperationalReportsView, error) {
	if client == nil {
		return FleetOperationalReportsView{}, ErrSessionNotConnected
	}
	if err := validateFleetDetailName(fleet); err != nil {
		return FleetOperationalReportsView{}, err
	}
	var state admin.FleetStateReport
	var stateErr error
	var schedules admin.FleetCronReport
	var schedulesErr error
	runWorkspaceTasks(ctx, 2, []func(context.Context){
		func(taskCtx context.Context) { state, stateErr = client.GetFleetStateReportContext(taskCtx, fleet) },
		func(taskCtx context.Context) {
			schedules, schedulesErr = client.GetFleetCronReportContext(taskCtx, fleet)
		},
	})
	if cause := context.Cause(ctx); cause != nil {
		return FleetOperationalReportsView{}, cause
	}
	if stateErr == nil && state.Fleet != fleet {
		stateErr = endpointDetailIdentityFailure("Fleet State report")
	}
	if schedulesErr == nil && schedules.Fleet != fleet {
		schedulesErr = endpointDetailIdentityFailure("Fleet schedule report")
	}
	states := make([]StateEvidence, 0, len(state.Endpoints))
	if stateErr == nil {
		if len(state.Endpoints) > readExportEndpointLimit {
			return FleetOperationalReportsView{}, errors.New("Fleet State report exceeds the supported Endpoint limit")
		}
		for _, report := range state.Endpoints {
			mapped, _ := mapEndpointDetailState(report, nil)
			states = append(states, mapped)
		}
	}
	rows := make([]FleetScheduleEvidence, 0)
	if schedulesErr == nil {
		if len(schedules.Endpoints) > readExportEndpointLimit {
			return FleetOperationalReportsView{}, errors.New("Fleet schedule report exceeds the supported Endpoint limit")
		}
		for _, report := range schedules.Endpoints {
			mapped, _ := mapEndpointDetailSchedules(report, nil)
			for _, schedule := range mapped {
				rows = append(rows, FleetScheduleEvidence{EndpointID: report.EndpointID, Name: schedule.Name, Schedule: schedule.Schedule, Applicable: schedule.Applicable, LastStatus: schedule.LastStatus, LastMessage: schedule.LastMessage, LastScheduledFor: schedule.LastScheduledFor, LastCompletedAt: schedule.LastCompletedAt})
			}
		}
	}
	slices.SortFunc(states, func(left, right StateEvidence) int { return strings.Compare(left.EndpointID, right.EndpointID) })
	slices.SortFunc(rows, func(left, right FleetScheduleEvidence) int {
		if compared := strings.Compare(left.EndpointID, right.EndpointID); compared != 0 {
			return compared
		}
		return strings.Compare(left.Name, right.Name)
	})
	loadedAt := s.now().UTC()
	return FleetOperationalReportsView{
		Fleet: fleet, States: states, Schedules: rows,
		Sections: FleetOperationalReportSections{
			State:     workspaceSectionResult("Fleet State evidence", stateErr, len(states), loadedAt, latestStateObservation([]admin.FleetStateReport{state})),
			Schedules: workspaceSectionResult("Fleet schedule evidence", schedulesErr, len(rows), loadedAt, latestFleetScheduleObservation(schedules)),
		},
	}, nil
}

func (s *ReadExportService) LoadFirewallReportConnected(ctx context.Context, client *admin.Client, endpointID string) (FirewallReportView, error) {
	if client == nil {
		return FirewallReportView{}, ErrSessionNotConnected
	}
	if err := validateEndpointDetailID(endpointID); err != nil {
		return FirewallReportView{}, err
	}
	var endpoint admin.Endpoint
	var endpointErr error
	var audit admin.FirewallAuditReport
	var auditErr error
	runWorkspaceTasks(ctx, 2, []func(context.Context){
		func(taskCtx context.Context) { endpoint, endpointErr = client.GetEndpointContext(taskCtx, endpointID) },
		func(taskCtx context.Context) {
			audit, auditErr = client.GetEndpointFirewallAuditContext(taskCtx, endpointID)
		},
	})
	if cause := context.Cause(ctx); cause != nil {
		return FirewallReportView{}, cause
	}
	if endpointErr != nil {
		return FirewallReportView{}, endpointErr
	}
	if endpoint.ID != endpointID {
		return FirewallReportView{}, endpointDetailIdentityFailure("Firewall Endpoint")
	}
	if auditErr == nil && audit.EndpointID != endpointID {
		auditErr = endpointDetailIdentityFailure("Firewall audit report")
	}
	mappedAudit, _, auditMapErr := mapEndpointDetailFirewall(audit, auditErr)
	if auditErr == nil {
		auditErr = auditMapErr
	}
	live, liveErr := mapFirewallLiveEvidence(endpoint.SystemInfo)
	loadedAt := s.now().UTC()
	return FirewallReportView{
		EndpointID: endpointID, Audit: mappedAudit, Live: live,
		Sections: FirewallReportSections{
			Audit: workspaceSectionResult("Firewall audit evidence", auditErr, len(mappedAudit), loadedAt, optionalTimePointer(audit.ReportedAt)),
			Live:  workspaceSectionResult("Live Firewall evidence", liveErr, firewallLiveItemCount(live), loadedAt, endpointSystemObservation(endpoint.SystemInfo)),
		},
	}, nil
}

func (s *ReadExportService) SaveFirewallReportConnected(ctx context.Context, client *admin.Client, request FirewallExportRequest) (ReadExportSaveResult, error) {
	format, err := validateReadExportFormat(request.Format)
	if err != nil {
		return ReadExportSaveResult{}, err
	}
	view, err := s.LoadFirewallReportConnected(ctx, client, request.EndpointID)
	if err != nil {
		return ReadExportSaveResult{}, err
	}
	rows := flattenFirewallExport(view.EndpointID, view.Live)
	if len(rows) == 0 {
		return ReadExportSaveResult{}, &ClassifiedError{Kind: ErrorUnavailable, Message: "No live Firewall evidence is available to export.", Guidance: "Refresh after the Endpoint reports its active Firewall configuration."}
	}
	var data []byte
	if format == "json" {
		data, err = json.MarshalIndent(rows, "", "  ")
		data = append(data, '\n')
	} else {
		data, err = encodeFirewallCSV(rows)
	}
	if err != nil {
		return ReadExportSaveResult{}, fmt.Errorf("encode Firewall report: %w", err)
	}
	return s.save(ctx, data, ReadExportDialogRequest{
		Title: "Save Firewall report", SuggestedName: request.EndpointID + "-firewall." + format,
		DisplayName: strings.ToUpper(format) + " (*." + format + ")", Pattern: "*." + format,
	})
}

func (s *ReadExportService) LoadAuditExportInfoConnected(ctx context.Context, client *admin.Client) (AuditExportInfoView, error) {
	if client == nil {
		return AuditExportInfoView{}, ErrSessionNotConnected
	}
	info, err := client.GetAuditExportInfoContext(ctx)
	if err != nil {
		return AuditExportInfoView{}, err
	}
	return AuditExportInfoView{ExportPath: info.ExportPath, PathKey: info.PathKey}, nil
}

func (s *ReadExportService) LoadDiagnosticRequestConnected(ctx context.Context, client *admin.Client, requestID string) (DiagnosticLifecycleView, error) {
	if client == nil {
		return DiagnosticLifecycleView{}, ErrSessionNotConnected
	}
	if strings.TrimSpace(requestID) == "" || strings.TrimSpace(requestID) != requestID {
		return DiagnosticLifecycleView{}, diagnosticBundleValidationFailure("Select one exact diagnostic request before loading lifecycle evidence.")
	}
	request, err := client.GetDiagnosticRequestContext(ctx, requestID)
	if err != nil {
		return DiagnosticLifecycleView{}, err
	}
	if request.ID != requestID {
		return DiagnosticLifecycleView{}, endpointDetailIdentityFailure("Diagnostic request")
	}
	return DiagnosticLifecycleView{
		RequestID: request.ID, EndpointID: request.EndpointID, RequestedBy: request.RequestedBy, Status: request.Status,
		Collectors: slices.Clone(request.Spec.Collectors), Since: formatTimestamp(request.Spec.Since), Until: formatTimestamp(request.Spec.Until),
		SHA256: request.SHA256, SizeBytes: request.SizeBytes, ErrorMessage: safeDiagnosticFailureText(request.Failure),
		CreatedAt: formatTimestamp(request.CreatedAt), DispatchedAt: formatOptionalReadExportTime(request.DispatchedAt),
		CompletedAt: formatOptionalReadExportTime(request.CompletedAt), ExpiresAt: formatTimestamp(request.ExpiresAt),
	}, nil
}

func safeDiagnosticFailureText(failure *executor.SafeError) string {
	if failure == nil {
		return ""
	}
	return boundedReadExportText(failure.Error(), 2048)
}

func (s *ReadExportService) save(ctx context.Context, data []byte, request ReadExportDialogRequest) (ReadExportSaveResult, error) {
	if s == nil || s.chooseDestination == nil {
		return ReadExportSaveResult{}, errors.New("native export save dialog is unavailable")
	}
	destination, err := s.chooseDestination(ctx, request)
	if err != nil {
		return ReadExportSaveResult{}, fmt.Errorf("choose native export destination: %w", err)
	}
	if destination == "" {
		return ReadExportSaveResult{Status: "canceled"}, nil
	}
	destination = filepath.Clean(destination)
	if filepath.Base(destination) == "." || filepath.Base(destination) == string(filepath.Separator) {
		return ReadExportSaveResult{}, errors.New("native export destination must name a file")
	}
	if err := writeReadExportAtomic(destination, data); err != nil {
		return ReadExportSaveResult{}, err
	}
	return ReadExportSaveResult{Status: "saved", Path: destination, SizeBytes: int64(len(data))}, nil
}

func writeReadExportAtomic(destination string, data []byte) error {
	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(directory, ".remotr-export-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary export: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("protect temporary export: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary export: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary export: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary export: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("place native export: %w", err)
	}
	if err := syncDiagnosticDirectory(directory); err != nil {
		_ = os.Remove(destination)
		return err
	}
	committed = true
	return nil
}

type assetSystemPayload struct {
	OSRelease struct{ PrettyName, Name, VersionID string } `json:"osRelease"`
	CPU       struct{ ModelName, CoreCount string }        `json:"cpu"`
	RAM       struct{ MemTotal string }                    `json:"ram"`
	Networks  []struct {
		Name, MACAddress string
		IPv4, IPv6       []string
	} `json:"networks"`
	BlockDevices []struct {
		Name, EncryptionType string
		Encrypted            bool
	} `json:"blockDevices"`
	Kernel struct{ Version string } `json:"kernel"`
	TPM    struct{ Version string } `json:"tpm"`
}

func mapAssetInventoryRow(endpoint admin.Endpoint) AssetInventoryRow {
	row := AssetInventoryRow{EndpointID: endpoint.ID, Fleet: endpoint.Fleet, AgentVersion: endpoint.ReportedAgentVersion}
	if endpoint.LastCheckIn != nil {
		row.LastCheckIn = formatTimestamp(endpoint.LastCheckIn.At)
	}
	if endpoint.SystemInfo == nil || len(endpoint.SystemInfo.Report) == 0 || len(endpoint.SystemInfo.Report) > readExportSystemReportLimit {
		return row
	}
	var payload assetSystemPayload
	if json.Unmarshal(endpoint.SystemInfo.Report, &payload) != nil {
		return row
	}
	switch {
	case payload.OSRelease.PrettyName != "":
		row.OS = payload.OSRelease.PrettyName
	case payload.OSRelease.Name != "" && payload.OSRelease.VersionID != "":
		row.OS = payload.OSRelease.Name + " " + payload.OSRelease.VersionID
	default:
		row.OS = payload.OSRelease.Name
	}
	row.CPU = payload.CPU.ModelName
	if row.CPU != "" && payload.CPU.CoreCount != "" {
		row.CPU += " (" + payload.CPU.CoreCount + " cores)"
	}
	row.RAM, row.Kernel = payload.RAM.MemTotal, payload.Kernel.Version
	for _, network := range payload.Networks {
		if network.Name == "lo" {
			continue
		}
		if row.MACAddress == "" {
			row.MACAddress = network.MACAddress
		}
		if row.PrimaryIP == "" && len(network.IPv4) > 0 {
			row.PrimaryIP = network.IPv4[0]
		}
	}
	if row.PrimaryIP == "" {
		for _, network := range payload.Networks {
			if network.Name != "lo" && len(network.IPv6) > 0 {
				row.PrimaryIP = network.IPv6[0]
				break
			}
		}
	}
	if len(payload.BlockDevices) > 0 {
		encrypted := 0
		for _, device := range payload.BlockDevices {
			if device.Encrypted {
				encrypted++
			}
		}
		row.DiskEncryption = fmt.Sprintf("%d/%d", encrypted, len(payload.BlockDevices))
	}
	if payload.TPM.Version == "" {
		row.TPM = "not reported"
	} else {
		row.TPM = "present (version " + payload.TPM.Version + ")"
	}
	row.EndpointID = boundedReadExportText(row.EndpointID, readExportTextLimit)
	row.Fleet = boundedReadExportText(row.Fleet, readExportTextLimit)
	row.OS = boundedReadExportText(row.OS, readExportTextLimit)
	row.CPU = boundedReadExportText(row.CPU, readExportTextLimit)
	row.RAM = boundedReadExportText(row.RAM, readExportTextLimit)
	row.Kernel = boundedReadExportText(row.Kernel, readExportTextLimit)
	row.PrimaryIP = boundedReadExportText(row.PrimaryIP, readExportTextLimit)
	row.MACAddress = boundedReadExportText(row.MACAddress, readExportTextLimit)
	row.DiskEncryption = boundedReadExportText(row.DiskEncryption, readExportTextLimit)
	row.TPM = boundedReadExportText(row.TPM, readExportTextLimit)
	row.AgentVersion = boundedReadExportText(row.AgentVersion, readExportTextLimit)
	return row
}

type firewallSystemPayload struct {
	Firewall struct {
		Backend   string `json:"backend"`
		Firewalld *struct {
			DefaultZone string `json:"defaultZone"`
			Zones       []struct {
				Name, Target             string
				Services, Ports, Sources []string
				RichRules                []string `json:"richRules"`
			} `json:"zones"`
		} `json:"firewalld"`
		Nftables *struct {
			RawRuleset string `json:"rawRuleset"`
		} `json:"nftables"`
	} `json:"firewall"`
}

func mapFirewallLiveEvidence(system *admin.SystemInfoSummary) (FirewallLiveEvidence, error) {
	if system == nil || len(system.Report) == 0 {
		return FirewallLiveEvidence{}, errors.New("Endpoint has no System evidence")
	}
	if len(system.Report) > readExportSystemReportLimit {
		return FirewallLiveEvidence{}, errors.New("Endpoint System evidence exceeds the supported report limit")
	}
	var payload firewallSystemPayload
	if err := json.Unmarshal(system.Report, &payload); err != nil {
		return FirewallLiveEvidence{}, fmt.Errorf("parse live Firewall evidence: %w", err)
	}
	live := FirewallLiveEvidence{Backend: boundedReadExportText(payload.Firewall.Backend, readExportTextLimit)}
	if payload.Firewall.Firewalld != nil {
		live.DefaultZone = boundedReadExportText(payload.Firewall.Firewalld.DefaultZone, readExportTextLimit)
		limit := min(len(payload.Firewall.Firewalld.Zones), readExportZoneLimit)
		for _, zone := range payload.Firewall.Firewalld.Zones[:limit] {
			live.Zones = append(live.Zones, FirewallZoneEvidence{
				Name: boundedReadExportText(zone.Name, readExportTextLimit), Target: boundedReadExportText(zone.Target, readExportTextLimit), Services: boundedStrings(zone.Services), Ports: boundedStrings(zone.Ports),
				Sources: boundedStrings(zone.Sources), RichRules: boundedStrings(zone.RichRules),
			})
		}
	}
	if payload.Firewall.Nftables != nil {
		live.Ruleset = payload.Firewall.Nftables.RawRuleset
		if len(live.Ruleset) > readExportRulesetLimit {
			live.Ruleset = live.Ruleset[:readExportRulesetLimit]
			live.RulesetTruncated = true
		}
	}
	if live.Backend == "" {
		return live, errors.New("Endpoint reported no live Firewall backend")
	}
	return live, nil
}

func boundedStrings(values []string) []string {
	bounded := slices.Clone(values[:min(len(values), readExportListLimit)])
	for index := range bounded {
		bounded[index] = boundedReadExportText(bounded[index], readExportTextLimit)
	}
	return bounded
}

func firewallLiveItemCount(live FirewallLiveEvidence) int {
	if live.Backend == "" {
		return 0
	}
	return max(1, len(live.Zones))
}

func latestFleetScheduleObservation(report admin.FleetCronReport) *time.Time {
	var latest time.Time
	for _, endpoint := range report.Endpoints {
		for _, job := range endpoint.Jobs {
			for _, observed := range []time.Time{job.LastScheduledFor, job.LastCompletedAt} {
				if observed.After(latest) {
					latest = observed
				}
			}
		}
	}
	return optionalTimePointer(latest)
}

type firewallExportRow struct {
	EndpointID string `json:"endpoint_id"`
	Backend    string `json:"backend"`
	Zone       string `json:"zone"`
	Target     string `json:"target"`
	Service    string `json:"service"`
	Port       string `json:"port"`
	Source     string `json:"source"`
	Ruleset    string `json:"raw_ruleset"`
}

func flattenFirewallExport(endpointID string, live FirewallLiveEvidence) []firewallExportRow {
	if live.Backend == "nftables" && live.Ruleset != "" {
		return []firewallExportRow{{EndpointID: endpointID, Backend: live.Backend, Ruleset: live.Ruleset}}
	}
	var rows []firewallExportRow
	for _, zone := range live.Zones {
		services, ports, sources := zone.Services, zone.Ports, zone.Sources
		if len(services) == 0 {
			services = []string{""}
		}
		if len(ports) == 0 {
			ports = []string{""}
		}
		if len(sources) == 0 {
			sources = []string{""}
		}
		for _, service := range services {
			for _, port := range ports {
				for _, source := range sources {
					rows = append(rows, firewallExportRow{EndpointID: endpointID, Backend: live.Backend, Zone: zone.Name, Target: zone.Target, Service: service, Port: port, Source: source})
				}
			}
		}
	}
	return rows
}

func encodeAssetInventoryCSV(rows []AssetInventoryRow) ([]byte, error) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	_ = writer.Write([]string{"endpoint_id", "fleet", "os", "cpu", "ram", "kernel", "primary_ip", "mac_address", "disk_encryption", "tpm", "agent_version", "last_check_in"})
	for _, row := range rows {
		_ = writer.Write([]string{row.EndpointID, row.Fleet, row.OS, row.CPU, row.RAM, row.Kernel, row.PrimaryIP, row.MACAddress, row.DiskEncryption, row.TPM, row.AgentVersion, row.LastCheckIn})
	}
	writer.Flush()
	return output.Bytes(), writer.Error()
}

func encodeFirewallCSV(rows []firewallExportRow) ([]byte, error) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	_ = writer.Write([]string{"endpoint_id", "backend", "zone", "target", "service", "port", "source", "raw_ruleset"})
	for _, row := range rows {
		_ = writer.Write([]string{row.EndpointID, row.Backend, row.Zone, row.Target, row.Service, row.Port, row.Source, row.Ruleset})
	}
	writer.Flush()
	return output.Bytes(), writer.Error()
}

func validateReadExportFormat(format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format != "csv" && format != "json" {
		return "", &ActionFailure{Kind: ActionValidation, Message: "The export format is invalid.", Guidance: "Choose CSV or JSON.", Retryable: false}
	}
	return format, nil
}

func formatOptionalReadExportTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTimestamp(*value)
}

func boundedReadExportText(value string, limit int) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
