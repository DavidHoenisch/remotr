package executor_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

func TestSafeSummaryRejectsRawSecretProjectionAtConstructionAndSerialization(t *testing.T) {
	unsafe := executor.SafeField{
		Path: "content", Sensitivity: executor.SafeSecret, Projection: executor.SafeValue, Text: "secret-canary",
	}
	if _, err := executor.NewSafeSummary([]executor.SafeField{unsafe}); err == nil {
		t.Fatal("NewSafeSummary accepted secret raw-value projection")
	}
	if _, err := json.Marshal(executor.SafeSummary{Fields: []executor.SafeField{unsafe}}); err == nil {
		t.Fatal("MarshalJSON accepted hand-built secret raw-value projection")
	}
}

func TestSafeErrorDiscardsProviderMessageAndCause(t *testing.T) {
	const canary = "provider-error-secret-canary"
	safe := executor.NewSafeError("apply_failed", "provider_apply", errors.New(canary))
	if strings.Contains(safe.Error(), canary) {
		t.Fatalf("safe error retained provider message: %q", safe.Error())
	}
	if errors.Unwrap(safe) != nil {
		t.Fatal("safe error exposed a raw provider cause")
	}
}

func TestSafeSummaryNormalizesCopiesRendersAndRoundTripsClassifiedFields(t *testing.T) {
	present := true
	count := 3
	fields := []executor.SafeField{
		{Path: "secret.ref", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: "secret/version/7"},
		{Path: "public.name", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: strings.Repeat("é", 300)},
		{Path: "metadata.present", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: &present},
		{Path: "metadata.count", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &count},
	}

	summary, err := executor.NewSafeSummary(fields)
	if err != nil {
		t.Fatal(err)
	}
	present = false
	count = 44
	if !*summary.Fields[1].Present || *summary.Fields[0].Count != 3 {
		t.Fatal("NewSafeSummary retained mutable projection pointers from its input")
	}
	if got := []string{summary.Fields[0].Path, summary.Fields[1].Path, summary.Fields[2].Path, summary.Fields[3].Path}; !reflect.DeepEqual(got, []string{"metadata.count", "metadata.present", "public.name", "secret.ref"}) {
		t.Fatalf("fields were not deterministically sorted: %v", got)
	}
	if got := summary.Fields[2].Text; len(got) > 512 || !utf8.ValidString(got) || got == fields[1].Text {
		t.Fatalf("long public projection was not safely truncated: bytes=%d valid=%t", len(got), utf8.ValidString(got))
	}
	if got := summary.String(); !strings.Contains(got, "metadata.count=3") || !strings.Contains(got, "metadata.present=true") || !strings.Contains(got, "secret.ref=secret/version/7") {
		t.Fatalf("unexpected safe rendering: %q", got)
	}

	clone := summary.Clone()
	*clone.Fields[0].Count = 99
	*clone.Fields[1].Present = false
	if *summary.Fields[0].Count != 3 || !*summary.Fields[1].Present {
		t.Fatal("Clone shared mutable projection pointers with the original")
	}

	wire, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip executor.SafeSummary
	if err := json.Unmarshal(wire, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(summary, roundTrip) {
		t.Fatalf("classified summary changed across JSON round trip:\nwant: %#v\n got: %#v", summary, roundTrip)
	}
	tied, err := executor.NewSafeSummary([]executor.SafeField{
		{Path: "same", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: "a"},
		{Path: "same", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{tied.Fields[0].Text, tied.Fields[1].Text}; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("equal-path fields were not sorted by projected text: %v", got)
	}
	stableTies, err := executor.NewSafeSummary([]executor.SafeField{
		{Path: "same", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: "same"},
		{Path: "same", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: "same"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := []executor.SafeSensitivity{stableTies.Fields[0].Sensitivity, stableTies.Fields[1].Sensitivity}; !reflect.DeepEqual(got, []executor.SafeSensitivity{executor.SafeSensitiveMetadata, executor.SafePublic}) {
		t.Fatalf("equal sort keys did not retain their stable input order: %v", got)
	}
	if got := (executor.SafeSummary{}).String(); got != "" {
		t.Fatalf("empty safe summary rendered %q", got)
	}
	if err := json.Unmarshal([]byte(`{"fields":`), &roundTrip); err == nil {
		t.Fatal("malformed classified summary JSON was accepted")
	}
	if err := json.Unmarshal([]byte(`{"fields":[{"path":"content","sensitivity":"secret","projection":"value","text":"secret-canary"}]}`), &roundTrip); err == nil {
		t.Fatal("classified summary JSON admitted a secret raw-value projection")
	}
}

func TestSafeSummaryRejectsInvalidClassificationAndProjectionShapes(t *testing.T) {
	present := true
	count := 1
	negative := -1
	cases := map[string]executor.SafeField{
		"missing path":             {Sensitivity: executor.SafePublic, Projection: executor.SafeValue},
		"public metadata":          {Path: "x", Sensitivity: executor.SafePublic, Projection: executor.SafeMetadata},
		"metadata raw value":       {Path: "x", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeValue, Text: "raw"},
		"secret fingerprint":       {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafeFingerprint, Text: "hash"},
		"unknown sensitivity":      {Path: "x", Sensitivity: "unknown", Projection: executor.SafeValue},
		"presence missing boolean": {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence},
		"presence with text":       {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence, Present: &present, Text: "raw"},
		"presence with count":      {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence, Present: &present, Count: &count},
		"count missing value":      {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafeCount},
		"negative count":           {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafeCount, Count: &negative},
		"count with text":          {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafeCount, Count: &count, Text: "raw"},
		"count with presence":      {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafeCount, Count: &count, Present: &present},
		"text with presence":       {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: "ref", Present: &present},
		"text with count":          {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: "ref", Count: &count},
		"invalid UTF-8":            {Path: "x", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: string([]byte{0xff})},
	}
	for name, field := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := executor.NewSafeSummary([]executor.SafeField{field}); err == nil {
				t.Fatalf("NewSafeSummary accepted %#v", field)
			}
		})
	}

	tooLong := executor.SafeSummary{Fields: []executor.SafeField{{
		Path: "public", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: strings.Repeat("x", 513),
	}}}
	if err := tooLong.Validate(); err == nil {
		t.Fatal("hand-built overlong safe field was accepted")
	}
	if _, err := json.Marshal(tooLong); err == nil {
		t.Fatal("hand-built overlong safe field was serialized")
	}
}

func TestSafeSummaryEnforcesClosedSensitivityProjectionMatrix(t *testing.T) {
	sensitivities := []executor.SafeSensitivity{executor.SafePublic, executor.SafeSensitiveMetadata, executor.SafeSecret}
	projections := []executor.SafeProjection{
		"aaaa",
		executor.SafeValue,
		executor.SafeMetadata,
		executor.SafeReference,
		executor.SafeFingerprint,
		executor.SafePresence,
		executor.SafeCount,
		"unknown",
		"zzzz",
	}
	allowed := map[executor.SafeSensitivity]map[executor.SafeProjection]bool{
		executor.SafePublic: {
			executor.SafeValue: true,
		},
		executor.SafeSensitiveMetadata: {
			executor.SafeMetadata: true, executor.SafeFingerprint: true, executor.SafePresence: true, executor.SafeCount: true,
		},
		executor.SafeSecret: {
			executor.SafeReference: true, executor.SafePresence: true, executor.SafeCount: true,
		},
	}
	for _, sensitivity := range sensitivities {
		for _, projection := range projections {
			t.Run(string(sensitivity)+"/"+string(projection), func(t *testing.T) {
				field := executor.SafeField{Path: "field", Sensitivity: sensitivity, Projection: projection, Text: "projection"}
				switch projection {
				case executor.SafePresence:
					present := true
					field.Text = ""
					field.Present = &present
				case executor.SafeCount:
					count := 1
					field.Text = ""
					field.Count = &count
				}
				_, err := executor.NewSafeSummary([]executor.SafeField{field})
				if allowed[sensitivity][projection] && err != nil {
					t.Fatalf("approved classification was rejected: %v", err)
				}
				if !allowed[sensitivity][projection] && err == nil {
					t.Fatalf("unapproved classification was accepted: %#v", field)
				}
			})
		}
	}
}

func TestSafeErrorValidatesRendersCancellationAndJSONAdmission(t *testing.T) {
	present := true
	details, err := executor.NewSafeSummary([]executor.SafeField{{
		Path: "credential.present", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence, Present: &present,
	}})
	if err != nil {
		t.Fatal(err)
	}

	canceled := executor.NewSafeErrorWithDetails("apply_failed", "provider_apply", context.Canceled, details)
	if err := canceled.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := canceled.Error(); got != "provider_apply canceled (apply_failed): credential.present=true" {
		t.Fatalf("unexpected canceled rendering: %q", got)
	}
	if !errors.Is(canceled, context.Canceled) || !errors.Is(canceled, context.DeadlineExceeded) || errors.Is(canceled, errors.New("other")) {
		t.Fatal("safe cancellation matching did not preserve the closed cancellation control flow")
	}
	*canceled.Details.Fields[0].Present = false
	if !*details.Fields[0].Present {
		t.Fatal("NewSafeErrorWithDetails retained mutable detail pointers")
	}

	failed := executor.NewSafeError("apply_failed", "provider_apply", errors.New("secret-canary"))
	if failed.Canceled {
		t.Fatal("ordinary provider error was marked canceled")
	}
	if got := failed.Error(); got != "provider_apply failed (apply_failed)" {
		t.Fatalf("unexpected failure rendering: %q", got)
	}
	deadline := executor.NewSafeError("apply_failed", "provider_apply", context.DeadlineExceeded)
	if !deadline.Canceled {
		t.Fatal("deadline cancellation was not retained as safe control-flow metadata")
	}

	wire, err := json.Marshal(failed)
	if err != nil {
		t.Fatal(err)
	}
	var decoded executor.SafeError
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(failed, decoded) {
		t.Fatalf("safe error changed across JSON round trip: want %#v, got %#v", failed, decoded)
	}
	if err := json.Unmarshal([]byte(`{"reasonCode":`), &decoded); err == nil {
		t.Fatal("malformed safe error JSON was accepted")
	}

	invalid := []executor.SafeError{
		{ReasonCode: "Invalid", Operation: "provider_apply"},
		{ReasonCode: "apply_failed", Operation: "Provider Apply"},
		{ReasonCode: "apply_failed", Operation: "provider_apply", Details: executor.SafeSummary{Fields: []executor.SafeField{{Path: "secret", Sensitivity: executor.SafeSecret, Projection: executor.SafeValue, Text: "secret-canary"}}}},
	}
	for _, unsafe := range invalid {
		if err := unsafe.Validate(); err == nil {
			t.Fatalf("Validate accepted unsafe error %#v", unsafe)
		}
		if _, err := json.Marshal(unsafe); err == nil {
			t.Fatalf("MarshalJSON accepted unsafe error %#v", unsafe)
		}
	}
	if err := json.Unmarshal([]byte(`{"reasonCode":"Invalid","operation":"provider_apply"}`), &decoded); err == nil {
		t.Fatal("UnmarshalJSON admitted an unstable reason code")
	}
}
