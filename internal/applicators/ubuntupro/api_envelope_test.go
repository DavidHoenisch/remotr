package ubuntupro

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const validEnvelope = `{"_schema_version":"v1","data":{"attributes":{},"meta":{"environment_vars":[]},"type":"Result"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`

// OS-UPM-039 and OS-UPM-040: the common envelope fails closed on ambiguous or
// unbounded structure while preserving stable codes and discarding messages.
func TestDecodeEnvelopeBoundariesAndStableErrors(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		valid bool
	}{
		{"valid", validEnvelope, true},
		{"unknown fields tolerated", strings.Replace(validEnvelope, `"result":"success"`, `"future":{"nested":true},"result":"success"`, 1), true},
		{"duplicate top-level member", strings.Replace(validEnvelope, `"result":"success"`, `"result":"failure","result":"success"`, 1), false},
		{"duplicate nested member", strings.Replace(validEnvelope, `"attributes":{}`, `"attributes":{"value":1,"value":2}`, 1), false},
		{"trailing JSON value", validEnvelope + `{}`, false},
		{"missing warnings", strings.Replace(validEnvelope, `,"warnings":[]`, ``, 1), false},
		{"missing errors", strings.Replace(validEnvelope, `"errors":[],`, ``, 1), false},
		{"missing attributes", strings.Replace(validEnvelope, `"attributes":{}`, `"future":{}`, 1), false},
		{"missing type", strings.Replace(validEnvelope, `,"type":"Result"`, ``, 1), false},
		{"invalid schema", strings.Replace(validEnvelope, `"_schema_version":"v1"`, `"_schema_version":"v2"`, 1), false},
		{"invalid result", strings.Replace(validEnvelope, `"result":"success"`, `"result":"maybe"`, 1), false},
		{"blank version", strings.Replace(validEnvelope, `"version":"32.3ubuntu0"`, `"version":""`, 1), false},
		{"invalid warning code", strings.Replace(validEnvelope, `"warnings":[]`, `"warnings":[{"code":"BAD CODE","msg":"translated","meta":{}}]`, 1), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeEnvelope([]byte(test.raw))
			if (err == nil) != test.valid {
				t.Fatalf("decodeEnvelope() error = %v, valid=%t", err, test.valid)
			}
		})
	}

	const localizedErrorCanary = "localized-api-error-message-canary"
	failure := strings.Replace(validEnvelope,
		`"errors":[],"result":"success"`,
		`"errors":[{"code":"invalid-token","msg":"`+localizedErrorCanary+`","meta":{"future":true}}],"result":"failure"`, 1)
	_, err := decodeEnvelope([]byte(failure))
	var apiError APIError
	if !errors.As(err, &apiError) || apiError.Code != "invalid-token" {
		t.Fatalf("decodeEnvelope() error = %T %v, want stable invalid-token APIError", err, err)
	}
	if strings.Contains(err.Error(), localizedErrorCanary) {
		t.Fatalf("localized error message escaped stable-code mapping: %v", err)
	}
}

func TestAPIClientRejectsInvalidEndpointAttributes(t *testing.T) {
	envelope := func(attributes string) []byte {
		return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":%s,"meta":{"environment_vars":[]},"type":"Result"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, attributes))
	}
	tests := []struct {
		name       string
		attributes string
		invoke     func(*APIClient) error
	}{
		{"version missing", `{}`, func(client *APIClient) error { _, err := client.Version(); return err }},
		{"attachment missing", `{}`, func(client *APIClient) error { _, err := client.IsAttached(); return err }},
		{"enabled unknown service", `{"enabled_services":[{"name":"future-beta","variant_enabled":false,"variant_name":null}]}`, func(client *APIClient) error { _, err := client.EnabledServices(); return err }},
		{"enabled invalid variant", `{"enabled_services":[{"name":"realtime-kernel","variant_enabled":true,"variant_name":"future"}]}`, func(client *APIClient) error { _, err := client.EnabledServices(); return err }},
		{"dependency lists missing", `{"services":[{"name":"usg"}]}`, func(client *APIClient) error { _, err := client.Dependencies(); return err }},
		{"attach unknown side effect", `{"enabled":["future-beta"],"reboot_required":false}`, func(client *APIClient) error { _, err := client.FullTokenAttach([]byte("token")); return err }},
		{"enable duplicate side effect", `{"enabled":["esm-apps","esm-apps"],"disabled":[],"reboot_required":false}`, func(client *APIClient) error { _, err := client.Enable("esm-apps", "", false); return err }},
		{"disable list missing", `{}`, func(client *APIClient) error { _, err := client.Disable("esm-apps", false); return err }},
		{"reboot enum unknown", `{"reboot_required":"later"}`, func(client *APIClient) error { _, err := client.RebootRequired(); return err }},
		{"detach reboot missing", `{"disabled":[]}`, func(client *APIClient) error { _, err := client.Detach(); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &apiBoundaryRunner{stdout: envelope(test.attributes), ordinaryStdout: envelope(test.attributes)}
			if err := test.invoke(NewAPIClient(runner)); err == nil {
				t.Fatal("invalid endpoint attributes were accepted")
			}
		})
	}
}

func FuzzDecodeEnvelopeBounded(f *testing.F) {
	f.Add([]byte(validEnvelope))
	f.Add([]byte(`{"_schema_version":"v1","result":"failure"}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > maxAPIOutputBytes {
			return
		}
		envelope, err := decodeEnvelope(raw)
		if err == nil {
			if envelope.SchemaVersion != "v1" || envelope.Result != "success" || envelope.Version == "" || envelope.Data.Type == "" || len(envelope.Data.Attributes) == 0 {
				t.Fatalf("accepted invalid envelope: %#v", envelope)
			}
		}
	})
}
