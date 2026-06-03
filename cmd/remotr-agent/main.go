package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/credentials"
	"github.com/DavidHoenisch/remotr/internal/agent/enroll"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/safepath"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
)

const defaultStateDir = "/var/lib/remotr"

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if len(os.Args) < 2 {
		runSyncLoop()
		return
	}

	switch os.Args[1] {
	case "enroll":
		os.Exit(runEnroll(os.Args[2:]))
	case "version", "-v", "--version":
		fmt.Printf("remotr-agent %s", version)
		if commit != "" {
			fmt.Printf(" (%s", commit)
			if date != "" {
				fmt.Printf(", %s", date)
			}
			fmt.Print(")")
		}
		fmt.Println()
		os.Exit(0)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runEnroll(args []string) int {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	serverURL := fs.String("server-url", "", "Remotr server base URL (default: REMOTR_SERVER_URL)")
	token := fs.String("token", "", "one-time enrollment token (default: REMOTR_ENROLL_TOKEN)")
	ca := fs.String("ca", "", "Remotr CA for server TLS trust (default: REMOTR_TLS_CA)")
	stateDir := fs.String("state-dir", "", "credential storage directory (default: REMOTR_STATE_DIR or /var/lib/remotr)")
	force := fs.Bool("force", false, "overwrite existing credentials in state-dir")
	endpointID := fs.String("endpoint-id", "", "endpoint identifier (default: REMOTR_ENDPOINT_ID or hostname-based)")
	serverKey := fs.Bool("server-key", false, "request server-generated private key (legacy; default uses local CSR)")
	noSync := fs.Bool("no-sync", false, "store credentials only; do not start sync loop")
	syncInterval := fs.Duration("sync-interval", 0, "sync interval after enroll (default: REMOTR_SYNC_INTERVAL or 30s)")

	_ = fs.Parse(args)

	base := strings.TrimSpace(firstNonEmpty(*serverURL, os.Getenv("REMOTR_SERVER_URL"), "https://remotr-server:8443"))
	tok := strings.TrimSpace(firstNonEmpty(*token, os.Getenv("REMOTR_ENROLL_TOKEN"), readEnrollTokenFile()))
	caPath := strings.TrimSpace(firstNonEmpty(*ca, os.Getenv("REMOTR_TLS_CA"), "/certs/ca.crt"))
	dir := strings.TrimSpace(firstNonEmpty(*stateDir, os.Getenv("REMOTR_STATE_DIR"), defaultStateDir))

	if tok == "" {
		fmt.Fprintln(os.Stderr, "enroll: token required (--token, REMOTR_ENROLL_TOKEN, or REMOTR_ENROLL_TOKEN_FILE)")
		return 2
	}
	if credentials.Present(dir) && !*force {
		fmt.Fprintf(os.Stderr, "enroll: credentials already exist in %s (use --force to replace)\n", dir)
		return 1
	}

	tlsCfg, err := tlsconfig.TrustOnlyTLSConfig(caPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll: tls config: %v\n", err)
		return 1
	}

	resolvedID, err := identity.ResolveEndpointID(firstNonEmpty(*endpointID, os.Getenv("REMOTR_ENDPOINT_ID")))
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll: endpoint id: %v\n", err)
		return 2
	}

	client := enroll.NewClient(base, tlsCfg)
	var resp enroll.Response
	if *serverKey {
		resp, err = client.EnrollWithServerKey(tok, resolvedID)
	} else {
		resp, err = client.Enroll(tok, resolvedID)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll: %v\n", err)
		return 1
	}

	if err := credentials.Save(dir, resp.EndpointID, resp.CertPEM, resp.KeyPEM, resp.CAPEM); err != nil {
		fmt.Fprintf(os.Stderr, "enroll: save credentials: %v\n", err)
		return 1
	}

	fmt.Printf("enrolled endpoint %s\n", resp.EndpointID)
	fmt.Printf("credentials stored in %s\n", dir)

	if *noSync {
		return 0
	}

	interval := *syncInterval
	if interval <= 0 {
		interval = envDurationOr("REMOTR_SYNC_INTERVAL", 30*time.Second)
	}
	return runSyncLoopWithTLS(dir, base, interval)
}

func runSyncLoop() {
	base := envOr("REMOTR_SERVER_URL", "https://remotr-server:8443")
	interval := envDurationOr("REMOTR_SYNC_INTERVAL", 30*time.Second)
	stateDir := envOr("REMOTR_STATE_DIR", defaultStateDir)

	if credentials.Present(stateDir) {
		os.Exit(runSyncLoopWithTLS(stateDir, base, interval))
	}

	cert := envOr("REMOTR_TLS_CERT", "/certs/agent.crt")
	key := envOr("REMOTR_TLS_KEY", "/certs/agent.key")
	ca := envOr("REMOTR_TLS_CA", "/certs/ca.crt")

	tlsCfg, err := tlsconfig.ClientTLSConfig(cert, key, ca)
	if err != nil {
		slog.Error("tls config", "err", err)
		os.Exit(1)
	}

	os.Exit(runSyncLoopWithConfig(base, tlsCfg, interval))
}

func runSyncLoopWithTLS(stateDir, base string, interval time.Duration) int {
	p, err := credentials.Layout(stateDir)
	if err != nil {
		slog.Error("credentials paths", "err", err)
		return 1
	}
	tlsCfg, err := tlsconfig.ClientTLSConfig(p.Cert, p.Key, p.CA)
	if err != nil {
		slog.Error("tls config", "err", err)
		return 1
	}
	return runSyncLoopWithConfig(base, tlsCfg, interval)
}

func runSyncLoopWithConfig(base string, tlsCfg *tls.Config, interval time.Duration) int {
	timeout := envDurationOr("REMOTR_SYNC_TIMEOUT", sync.DefaultHTTPTimeout)
	client := sync.NewClientWithTimeout(base, tlsCfg, timeout)
	var state syncRunState
	var pending sync.Pending

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	state.runOnce(ctx, client, &pending, version)
	for {
		select {
		case <-ctx.Done():
			return 0
		case <-ticker.C:
			state.runOnce(ctx, client, &pending, version)
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func readEnrollTokenFile() string {
	path := strings.TrimSpace(os.Getenv("REMOTR_ENROLL_TOKEN_FILE"))
	if path == "" {
		return ""
	}
	b, err := safepath.ReadConfigFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func usage() {
	fmt.Fprintf(os.Stderr, `remotr-agent — Remotr endpoint agent

Usage:
  remotr-agent                     Run sync loop (default)
  remotr-agent enroll [flags]      Exchange enrollment token for credentials
  remotr-agent help

Default sync uses credentials from REMOTR_STATE_DIR when present, otherwise
REMOTR_TLS_CERT / REMOTR_TLS_KEY / REMOTR_TLS_CA.

Enroll flags:
  -server-url string    Remotr server base URL
  -token string         one-time enrollment token
  -ca string            Remotr CA for server TLS trust
  -state-dir string     credential directory (default /var/lib/remotr)
  -force                overwrite existing credentials
  -endpoint-id string   endpoint identifier (or REMOTR_ENDPOINT_ID)
  -server-key           use server-generated key (legacy; default is local CSR)
  -no-sync              store credentials only
  -sync-interval        sync interval after enroll

Examples:
  remotr-agent enroll --token "$TOKEN" --server-url https://remotr.example:8443 --ca /etc/remotr/ca.crt
  REMOTR_STATE_DIR=/var/lib/remotr remotr-agent

`)
}
