// Package performance provides opt-in, controlled-environment diagnostics for
// benchmark, load, and soak evidence. It never exposes request or secret data.
package performance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	secretCanaryPattern    = regexp.MustCompile(`(?i)remotr-test-secret-[0-9a-f]+`)
	authorizationPattern   = regexp.MustCompile(`(?im)^authorization:[^\r\n]*`)
	bearerPattern          = regexp.MustCompile(`(?i)bearer\s+[^\s]+`)
	urlCredentialPattern   = regexp.MustCompile(`([a-z][a-z0-9+.-]*://)[^/@:\s]+:[^/@\s]+@`)
	credentialFieldPattern = regexp.MustCompile(`(?i)(password|token|secret|private[_-]?key)\s*[=:]\s*[^\s,;]+`)
)

// RuntimeMetrics is a bounded point-in-time server runtime snapshot.
type RuntimeMetrics struct {
	HeapAllocBytes uint64 `json:"heapAllocBytes"`
	HeapObjects    uint64 `json:"heapObjects"`
	Goroutines     int    `json:"goroutines"`
	GCCycles       uint32 `json:"gcCycles"`
}

// NewDiagnosticsHandler returns the controlled diagnostics surface.
func NewDiagnosticsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/remotr/metrics", func(response http.ResponseWriter, _ *http.Request) {
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(RuntimeMetrics{
			HeapAllocBytes: memory.HeapAlloc,
			HeapObjects:    memory.HeapObjects,
			Goroutines:     runtime.NumGoroutine(),
			GCCycles:       memory.NumGC,
		})
	})
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	for _, profile := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		mux.Handle("/debug/pprof/"+profile, pprof.Handler(profile))
	}
	return mux
}

// StartDiagnostics starts the aggregate and pprof surface on an explicitly
// loopback-only address and shuts it down with ctx.
func StartDiagnostics(ctx context.Context, address string) error {
	if err := ValidateDiagnosticsAddress(address); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen for performance diagnostics: %w", err)
	}
	diagnostics := &http.Server{
		Addr:              address,
		Handler:           NewDiagnosticsHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = diagnostics.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := diagnostics.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("performance diagnostics listener", "err", err)
		}
	}()
	return nil
}

// ValidateDiagnosticsAddress prevents the opt-in diagnostics listener from
// becoming a remotely reachable administration or profiling surface.
func ValidateDiagnosticsAddress(address string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return fmt.Errorf("diagnostics address: %w", err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("diagnostics address has invalid port %q", port)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("diagnostics address must use an explicit loopback host")
	}
	return nil
}

// SanitizeDiagnostic bounds retained textual evidence and removes common
// credential forms plus the repository's synthetic secret canaries.
func SanitizeDiagnostic(input []byte, limit int) []byte {
	if limit <= 0 {
		return nil
	}
	text := string(input)
	text = secretCanaryPattern.ReplaceAllString(text, "[redacted]")
	text = authorizationPattern.ReplaceAllString(text, "Authorization: [redacted]")
	text = bearerPattern.ReplaceAllString(text, "[redacted]")
	text = urlCredentialPattern.ReplaceAllString(text, `${1}[redacted]@`)
	text = credentialFieldPattern.ReplaceAllString(text, `${1}=[redacted]`)
	if len(text) > limit {
		text = text[:limit]
	}
	return []byte(text)
}
