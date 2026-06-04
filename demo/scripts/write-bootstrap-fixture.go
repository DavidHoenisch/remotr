//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: write-bootstrap-fixture <out.json> <cert.crt> <key.key> <ca.crt>")
		os.Exit(2)
	}
	certPEM, err := os.ReadFile(os.Args[2])
	if err != nil {
		fatal(err)
	}
	keyPEM, err := os.ReadFile(os.Args[3])
	if err != nil {
		fatal(err)
	}
	caPEM, err := os.ReadFile(os.Args[4])
	if err != nil {
		fatal(err)
	}

	body := map[string]string{
		"operator_id": "demo-operator",
		"cert_pem":    string(certPEM),
		"key_pem":     string(keyPEM),
		"ca_pem":      string(caPEM),
	}
	bodyRaw, err := json.Marshal(body)
	if err != nil {
		fatal(err)
	}

	out := map[string]any{
		"status": 200,
		"body":   json.RawMessage(bodyRaw),
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(os.Args[1], raw, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
