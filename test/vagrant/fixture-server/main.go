package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	advertise := flag.String("advertise", "", "routable host IPv4 address advertised to the VM")
	result := flag.String("result", "", "path written after a valid recovery acknowledgement")
	flag.Parse()

	token := os.Getenv("REMOTR_VM_FIXTURE_TOKEN")
	if *advertise == "" || *result == "" || token == "" {
		fmt.Fprintln(os.Stderr, "advertise, result, and REMOTR_VM_FIXTURE_TOKEN are required")
		os.Exit(2)
	}

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		fmt.Fprintln(os.Stderr, "listener port:", err)
		os.Exit(1)
	}

	server := &http.Server{Handler: recoveryHandler(token, *result)}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "serve:", err)
		}
	}()

	fmt.Printf("READY http://%s:%s\n", *advertise, port)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func recoveryHandler(token, result string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			_, _ = w.Write([]byte("ok\n"))
		case "/ack":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if err := os.MkdirAll(filepath.Dir(result), 0o700); err != nil {
				http.Error(w, "result directory", http.StatusInternalServerError)
				return
			}
			if err := os.WriteFile(result, []byte("acknowledged\n"), 0o600); err != nil {
				http.Error(w, "result", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}
