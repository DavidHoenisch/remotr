// remotr-load drives opt-in authenticated Sync load against disposable infrastructure.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/loadtest"
)

func main() {
	var cfg loadtest.Config
	allow := flag.Bool("allow-load", false, "confirm a disposable load environment")
	flag.StringVar(&cfg.ServerURL, "server-url", os.Getenv("REMOTR_LOAD_SERVER_URL"), "Remotr server URL")
	flag.StringVar(&cfg.CAPath, "ca", os.Getenv("REMOTR_LOAD_CA"), "CA certificate path")
	flag.StringVar(&cfg.DatabaseURL, "database-url", os.Getenv("REMOTR_LOAD_DATABASE_URL"), "disposable Postgres URL")
	flag.StringVar(&cfg.Fleet, "fleet", os.Getenv("REMOTR_LOAD_FLEET"), "preconfigured disposable fleet")
	flag.StringVar(&cfg.RunID, "run-id", os.Getenv("REMOTR_LOAD_RUN_ID"), "unique load-run prefix")
	flag.IntVar(&cfg.EndpointCount, "endpoints", 0, "number of unique endpoints")
	flag.IntVar(&cfg.Concurrency, "concurrency", 0, "maximum simultaneous Sync requests")
	flag.DurationVar(&cfg.RequestTimeout, "request-timeout", 30*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.EnrollmentTTL, "enrollment-ttl", time.Hour, "one-time enrollment token lifetime")
	flag.Parse()

	if !*allow {
		fmt.Fprintln(os.Stderr, "refusing load: pass --allow-load only for disposable test infrastructure")
		os.Exit(2)
	}
	harness, err := loadtest.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load configuration:", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := harness.Provision(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "provision load endpoints:", err)
		os.Exit(1)
	}
	summary := harness.SyncWave(ctx, sync.Request{})
	if err := json.NewEncoder(os.Stdout).Encode(summary); err != nil {
		fmt.Fprintln(os.Stderr, "encode load summary:", err)
		os.Exit(1)
	}
	if summary.Errors > 0 {
		os.Exit(1)
	}
}
