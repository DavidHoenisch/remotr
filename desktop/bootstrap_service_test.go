package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
)

const bootstrapTokenCanary = "bootstrap-token-canary-do-not-retain"

func TestBootstrapServicePersistsCredentialAndVerifiesIdentity(t *testing.T) {
	fixture := newConnectionTLSFixture(t)
	issuedCertPEM, issuedKeyPEM := issueConnectionCertificate(
		t,
		fixture.caCertificate,
		fixture.caKey,
		big.NewInt(40),
		"operator-issued-by-bootstrap",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		false,
	)
	response := admin.BootstrapResponse{
		OperatorID: "operator-issued-by-bootstrap",
		CertPEM:    string(issuedCertPEM),
		KeyPEM:     string(issuedKeyPEM),
		CAPEM:      string(fixture.caPEM),
	}
	var receivedToken string
	server := startBootstrapTLSServer(t, fixture, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/admin/bootstrap":
			var input admin.BootstrapRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				http.Error(writer, "bad request", http.StatusBadRequest)
				return
			}
			receivedToken = input.Token
			writeBootstrapJSON(t, writer, http.StatusOK, response)
		case "/v1/admin/me":
			if request.TLS == nil || len(request.TLS.PeerCertificates) == 0 {
				http.Error(writer, "missing client certificate", http.StatusForbidden)
				return
			}
			writeBootstrapJSON(t, writer, http.StatusOK, admin.OperatorMe{
				OperatorID: "operator-verified-after-bootstrap",
				Roles:      []string{"operator"},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	attempt := newBootstrapAttempt(t, server.URL, fixture.caPEM)
	stateDir := attempt.Profile.StateDir

	got, err := NewBootstrapService().Bootstrap(t.Context(), attempt)
	if err != nil {
		t.Fatalf("bootstrap profile: %v", err)
	}
	want := ConnectionView{
		ProfileName: attempt.Profile.Name,
		ServerURL:   server.URL,
		OperatorID:  "operator-verified-after-bootstrap",
		Roles:       []string{"operator"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bootstrap connection view = %#v, want %#v", got, want)
	}
	if receivedToken != bootstrapTokenCanary {
		t.Fatalf("bootstrap API token = %q, want synthetic canary", receivedToken)
	}
	assertBootstrapTokenCleared(t, attempt.Token)

	layout, err := opcreds.Layout(stateDir)
	if err != nil {
		t.Fatalf("inspect persisted credential layout: %v", err)
	}
	for _, path := range []string{layout.Cert, layout.Key, layout.CA, layout.Meta} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat persisted credential %s: %v", filepath.Base(path), err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("persisted credential %s mode = %04o, want 0600", filepath.Base(path), info.Mode().Perm())
		}
	}
	persistedKey, err := os.ReadFile(layout.Key)
	if err != nil {
		t.Fatalf("read persisted private key: %v", err)
	}
	if string(persistedKey) != string(issuedKeyPEM) {
		t.Fatal("persisted private key does not match the issued credential")
	}
	resultJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("encode bootstrap result: %v", err)
	}
	for _, canary := range []string{bootstrapTokenCanary, string(issuedKeyPEM), string(issuedCertPEM)} {
		if strings.Contains(string(resultJSON), canary) {
			t.Errorf("bootstrap result disclosed secret credential material")
		}
	}
}

func TestBootstrapServiceRejectsAPIErrorWithoutPersistence(t *testing.T) {
	fixture := newConnectionTLSFixture(t)
	const rejectionCanary = "bootstrap-rejection-body-secret-canary"
	server := startBootstrapTLSServer(t, fixture, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input admin.BootstrapRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		http.Error(writer, rejectionCanary+" "+input.Token, http.StatusForbidden)
	}))
	attempt := newBootstrapAttempt(t, server.URL, fixture.caPEM)
	stateDir := attempt.Profile.StateDir

	view, err := NewBootstrapService().Bootstrap(t.Context(), attempt)
	if !reflect.DeepEqual(view, ConnectionView{}) {
		t.Fatalf("rejected bootstrap returned a connection view: %#v", view)
	}
	assertBootstrapFailure(t, err, BootstrapRejected, stateDir, bootstrapTokenCanary, rejectionCanary)
	assertBootstrapTokenCleared(t, attempt.Token)
	assertNoCredentialFragments(t, stateDir)
}

func TestBootstrapServiceCleansPartialPersistenceFailure(t *testing.T) {
	fixture := newConnectionTLSFixture(t)
	issuedCertPEM, issuedKeyPEM := issueConnectionCertificate(
		t,
		fixture.caCertificate,
		fixture.caKey,
		big.NewInt(41),
		"operator-partial-write",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		false,
	)
	response := admin.BootstrapResponse{
		OperatorID: "operator-partial-write",
		CertPEM:    string(issuedCertPEM),
		KeyPEM:     string(issuedKeyPEM),
		CAPEM:      string(fixture.caPEM),
	}
	server := startBootstrapTLSServer(t, fixture, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeBootstrapJSON(t, writer, http.StatusOK, response)
	}))
	attempt := newBootstrapAttempt(t, server.URL, fixture.caPEM)
	stateDir := attempt.Profile.StateDir
	const persistenceCanary = "synthetic-persistence-error-secret-canary"
	persistPartial := func(dir, _ string, certPEM, keyPEM, _ string) error {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		layout, err := opcreds.Layout(dir)
		if err != nil {
			return err
		}
		if err := os.WriteFile(layout.Cert, []byte(certPEM), 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(layout.Key, []byte(keyPEM), 0o600); err != nil {
			return err
		}
		return errors.New(persistenceCanary)
	}

	view, err := NewBootstrapService(WithCredentialPersistence(persistPartial)).Bootstrap(t.Context(), attempt)
	if !reflect.DeepEqual(view, ConnectionView{}) {
		t.Fatalf("failed persistence returned a connection view: %#v", view)
	}
	assertBootstrapFailure(t, err, BootstrapPersistenceFailed, stateDir, bootstrapTokenCanary, persistenceCanary, string(issuedKeyPEM))
	assertBootstrapTokenCleared(t, attempt.Token)
	assertNoCredentialFragments(t, stateDir)
}

func TestBootstrapServiceCancellationClearsTransientState(t *testing.T) {
	fixture := newConnectionTLSFixture(t)
	requestStarted := make(chan struct{})
	requestCancelled := make(chan struct{})
	server := startBootstrapTLSServer(t, fixture, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
		close(requestCancelled)
	}))
	attempt := newBootstrapAttempt(t, server.URL, fixture.caPEM)
	stateDir := attempt.Profile.StateDir
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := NewBootstrapService().Bootstrap(ctx, attempt)
		result <- err
	}()
	<-requestStarted
	cancel()
	<-requestCancelled

	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled bootstrap error = %v, want context.Canceled", err)
	}
	assertBootstrapTokenCleared(t, attempt.Token)
	assertNoCredentialFragments(t, stateDir)
}

func newBootstrapAttempt(t *testing.T, serverURL string, caPEM []byte) *BootstrapAttempt {
	t.Helper()
	root := t.TempDir()
	caPath := filepath.Join(root, "bootstrap-ca.crt")
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatalf("write bootstrap trust fixture: %v", err)
	}
	return &BootstrapAttempt{
		Profile: ConnectionProfile{
			Name:      "Bootstrap profile",
			ServerURL: serverURL,
			StateDir:  filepath.Join(root, "operator-state"),
			CAPath:    caPath,
		},
		Token: []byte(bootstrapTokenCanary),
	}
}

func startBootstrapTLSServer(t *testing.T, fixture connectionTLSFixture, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{fixture.serverCert},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ClientCAs:    connectionCertPool(t, fixture.caPEM),
		MinVersion:   tls.VersionTLS12,
		Time: func() time.Time {
			return connectionTestTime
		},
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func writeBootstrapJSON(t *testing.T, writer http.ResponseWriter, status int, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("write controlled bootstrap response: %v", err)
	}
}

func assertBootstrapFailure(t *testing.T, err error, kind BootstrapFailureKind, stateDir string, canaries ...string) {
	t.Helper()
	var failure *BootstrapFailure
	if !errors.As(err, &failure) {
		t.Fatalf("bootstrap error = %v, want BootstrapFailure", err)
	}
	if failure.Kind != kind {
		t.Fatalf("bootstrap failure kind = %q, want %q", failure.Kind, kind)
	}
	if strings.TrimSpace(failure.Message) == "" || strings.TrimSpace(failure.Guidance) == "" {
		t.Fatalf("bootstrap failure lacks safe message or guidance: %#v", failure)
	}
	safeText := failure.Error() + " " + failure.Guidance
	for _, forbidden := range append(canaries, stateDir, filepath.Join(stateDir, "operator.key"), "BEGIN PRIVATE KEY", "BEGIN EC PRIVATE KEY") {
		if forbidden != "" && strings.Contains(safeText, forbidden) {
			t.Errorf("bootstrap failure disclosed forbidden value %q: %s", forbidden, safeText)
		}
	}
}

func assertBootstrapTokenCleared(t *testing.T, token []byte) {
	t.Helper()
	for index, value := range token {
		if value != 0 {
			t.Fatalf("transient bootstrap token byte %d was not cleared", index)
		}
	}
}

func assertNoCredentialFragments(t *testing.T, stateDir string) {
	t.Helper()
	layout, err := opcreds.Layout(stateDir)
	if err != nil {
		t.Fatalf("inspect credential layout: %v", err)
	}
	for _, path := range []string{layout.Cert, layout.Key, layout.CA, layout.Meta} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("credential fragment remains at %s: %v", filepath.Base(path), err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(stateDir), ".operator-state-*"))
	if err != nil {
		t.Fatalf("inspect credential staging directories: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("credential staging artifacts remain: %v", matches)
	}
}
