package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseCanonicalCertificateRequiresReferenceOnlyProviderMaterial(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: security
    resources:
      - kind: certificate
        name: service
        certificatePath: /etc/service/tls.crt
        privateKeyPath: /etc/service/tls.key
        certificateRef: remotr:certificates/service@active
        privateKeyRef: remotr:private-keys/service@7
        chainRefs: [local-file:/run/secrets/service-chain.pem]
        subject: CN=service.example.test
        sans: [service.example.test]
        renewBefore: 720h
        renewalPolicy: provider
        owner: root
        group: service
        certificateMode: [416]
        privateKeyMode: [384]
        notifications:
          - type: reload
            target: service.service
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].Certificates) != 1 {
		t.Fatalf("parsed state = %#v", state)
	}
	resource := state.Configurations[0].Certificates[0]
	if resource.Kind != models.ResourceKindCertificate || resource.PrivateKeyRef != "remotr:private-keys/service@7" || resource.RenewalPolicy != models.CertificateRenewalProvider {
		t.Fatalf("certificate = %#v", resource)
	}

	_, err = models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: security
    resources:
      - kind: certificate
        name: service
        certificatePath: /etc/service/tls.crt
        privateKeyPath: /etc/service/tls.key
        certificateRef: remotr:certificates/service@active
        privateKey: PRIVATE-KEY-CANARY
`))
	if err == nil || !strings.Contains(err.Error(), "field privateKey not found") {
		t.Fatalf("inline private key error = %v", err)
	}
}
