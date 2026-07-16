package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDesktopViewModelStatusVocabularyIsExplicit(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "section loading", got: string(SectionLoading), want: "loading"},
		{name: "section partial", got: string(SectionPartial), want: "partial"},
		{name: "section unavailable", got: string(SectionUnavailable), want: "unavailable"},
		{name: "compliant", got: string(ComplianceCompliant), want: "compliant"},
		{name: "check failed", got: string(ComplianceCheckFailed), want: "check_failed"},
		{name: "not reported", got: string(ComplianceNotReported), want: "not_reported"},
		{name: "freshness recent", got: string(FreshnessRecent), want: "recent"},
		{name: "freshness stale", got: string(FreshnessStale), want: "stale"},
		{name: "freshness never reported", got: string(FreshnessNeverReported), want: "never_reported"},
		{name: "authorization error", got: string(ErrorAuthorization), want: "authorization"},
		{name: "accepted action", got: string(ActionAccepted), want: "accepted"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("status = %q, want %q", test.got, test.want)
			}
			for _, forbidden := range []string{"online", "offline"} {
				if strings.Contains(test.got, forbidden) {
					t.Errorf("status %q implies unsupported connection presence", test.got)
				}
			}
		})
	}
}

func TestDesktopViewModelsAreTypedBoundedAndCredentialFree(t *testing.T) {
	now := time.Date(2032, time.March, 4, 5, 6, 7, 0, time.UTC)
	models := []any{
		OperatorView{OperatorID: "operator-1", Roles: []string{"read_only"}},
		SectionResult{
			State: SectionReady,
			Snapshot: SnapshotTimestamps{
				LoadedAt:   now,
				ObservedAt: &now,
			},
		},
		EndpointRow{
			EndpointID:           "endpoint-1",
			Fleet:                "production",
			Usernames:            []string{"alice"},
			Compliance:           ComplianceCompliant,
			Freshness:            FreshnessRecent,
			DesiredAgentVersion:  "v2.0.0",
			ReportedAgentVersion: "v2.0.0",
			ReleaseRef:           "release-42",
			Labels:               []LabelView{{Key: "region", Value: "west"}},
			EvidenceAt:           &now,
		},
		FleetSummary{
			Fleet:         "production",
			EndpointCount: 1,
			Compliance:    []StatusCount{{Status: string(ComplianceCompliant), Count: 1}},
			Freshness:     []StatusCount{{Status: string(FreshnessRecent), Count: 1}},
			AgentVersions: []StatusCount{{Status: "v2.0.0", Count: 1}},
		},
		StateEvidence{
			EndpointID: "endpoint-1",
			ReleaseRef: "release-42",
			Status:     ComplianceCompliant,
			ReportedAt: now,
			Items: []StateEvidenceItem{{
				Address:         "packages/curl",
				Name:            "curl",
				Provider:        "packages",
				Status:          ComplianceCompliant,
				DesiredSummary:  "installed",
				ObservedSummary: "installed",
			}},
		},
		ChangeRequestSummary{
			ChangeRequestID:   "change-1",
			Fleet:             "production",
			ReleaseRef:        "release-42",
			Risk:              "standard",
			Lifecycle:         "pending",
			TargetCount:       1,
			RequiredApprovals: 1,
			ApprovalCount:     0,
			UpdatedAt:         now,
		},
		ActivityEvent{
			EventID:      "event-1",
			OccurredAt:   now,
			Actor:        "operator-1",
			Action:       "git_sync",
			ResourceType: "server",
			ResourceID:   "primary",
			Status:       "accepted",
			RequestID:    "request-1",
			Details:      []ActivityDetail{{Key: "releaseRef", Value: "release-42"}},
		},
		ActionResult{
			Action:     "git_sync",
			Target:     "production-profile",
			Status:     ActionAccepted,
			Message:    "The server accepted the Git sync request.",
			RequestID:  "request-1",
			AcceptedAt: now,
		},
		ClassifiedError{
			Kind:     ErrorAuthorization,
			Message:  "The current Operator is not authorized for this section.",
			Guidance: "Ask an administrator to review assigned roles.",
		},
		SnapshotTimestamps{LoadedAt: now, ObservedAt: &now, FailedAt: &now},
	}

	for _, model := range models {
		t.Run(reflect.TypeOf(model).Name(), func(t *testing.T) {
			assertBoundedSafeViewType(t, reflect.TypeOf(model), map[reflect.Type]bool{})
		})
	}

	encoded, err := json.Marshal(models)
	if err != nil {
		t.Fatalf("encode desktop view models: %v", err)
	}
	for _, forbidden := range []string{
		"model-private-key-canary",
		"model-bootstrap-token-canary",
		"model-certificate-canary",
		"BEGIN PRIVATE KEY",
		"BEGIN CERTIFICATE",
		"certFingerprint",
		"clientIp",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("desktop view models disclosed forbidden value or field %q: %s", forbidden, encoded)
		}
	}
}

func assertBoundedSafeViewType(t *testing.T, modelType reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	for modelType.Kind() == reflect.Pointer || modelType.Kind() == reflect.Slice || modelType.Kind() == reflect.Array {
		if modelType.Kind() == reflect.Slice && modelType.Elem().Kind() == reflect.Uint8 {
			t.Errorf("view model contains raw byte slice %s", modelType)
			return
		}
		modelType = modelType.Elem()
	}
	if modelType == reflect.TypeFor[time.Time]() || seen[modelType] {
		return
	}
	switch modelType.Kind() {
	case reflect.Map, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		t.Errorf("view model contains unbounded field type %s", modelType)
		return
	case reflect.Struct:
		seen[modelType] = true
	default:
		return
	}

	for index := 0; index < modelType.NumField(); index++ {
		field := modelType.Field(index)
		normalizedName := strings.ToLower(field.Name)
		for _, forbidden := range []string{
			"token", "privatekey", "keypem", "certpem", "certificate", "fingerprint",
			"secret", "httpclient", "tlsconfig", "raw", "diagnosticbytes", "clientip",
		} {
			if strings.Contains(normalizedName, forbidden) {
				t.Errorf("view model %s contains forbidden field %s", modelType.Name(), field.Name)
			}
		}
		if field.IsExported() {
			jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
			if jsonName == "" || jsonName == "-" {
				t.Errorf("view model %s.%s lacks a stable JSON field name", modelType.Name(), field.Name)
			}
		}
		assertBoundedSafeViewType(t, field.Type, seen)
	}
}
