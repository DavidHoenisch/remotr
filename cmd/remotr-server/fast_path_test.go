package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFastPathBackendSelection(t *testing.T) {
	tests := []struct {
		name, backend, redisURL, prefix, legacy string
		processes                               int
		wantBackend                             string
		wantErr                                 string
	}{
		{name: "default memory", wantBackend: "memory"},
		{name: "disabled", backend: "disabled", wantBackend: "disabled"},
		{name: "memory", backend: "memory", wantBackend: "memory"},
		{name: "redis", backend: "redis", redisURL: "rediss://:secret@example.invalid:6380", prefix: "prod-a", processes: 2, wantBackend: "redis"},
		{name: "legacy rollback", backend: "redis", redisURL: "redis://:secret@example.invalid:6379", prefix: "prod-a", legacy: "false", wantBackend: "disabled"},
		{name: "redis missing url", backend: "redis", prefix: "prod-a", wantErr: "REMOTR_REDIS_URL"},
		{name: "redis missing prefix", backend: "redis", redisURL: "redis://:secret@example.invalid:6379", wantErr: "REMOTR_UNCHANGED_SYNC_REDIS_PREFIX"},
		{name: "invalid backend", backend: "disk", wantErr: "REMOTR_UNCHANGED_SYNC_BACKEND"},
		{name: "memory multiple processes", backend: "memory", processes: 2, wantErr: "memory backend requires one serving process"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{
				"REMOTR_UNCHANGED_SYNC_BACKEND":      test.backend,
				"REMOTR_REDIS_URL":                   test.redisURL,
				"REMOTR_UNCHANGED_SYNC_REDIS_PREFIX": test.prefix,
				"REMOTR_UNCHANGED_SYNC_FAST_PATH":    test.legacy,
			}
			if test.processes > 0 {
				values["REMOTR_SERVER_PROCESSES"] = fmt.Sprint(test.processes)
			}
			config, err := fastPathConfigFromEnvironment(func(key string) string { return values[key] })
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				if err != nil && strings.Contains(err.Error(), "secret") {
					t.Fatalf("error leaked Redis credential: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(config.Backend) != test.wantBackend {
				t.Fatalf("backend = %q, want %q", config.Backend, test.wantBackend)
			}
		})
	}
}

func TestFastPathConfigFromEnvironmentTopologyBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		values      map[string]string
		wantEnabled bool
		wantServing int
		wantErr     bool
	}{
		{name: "default memory", values: map[string]string{}, wantEnabled: true, wantServing: 1},
		{name: "single process enabled", values: map[string]string{"REMOTR_UNCHANGED_SYNC_FAST_PATH": "true"}, wantEnabled: true, wantServing: 1},
		{name: "multiple memory processes rejected", values: map[string]string{"REMOTR_UNCHANGED_SYNC_FAST_PATH": "true", "REMOTR_SERVER_PROCESSES": "2"}, wantErr: true},
		{name: "invalid boolean", values: map[string]string{"REMOTR_UNCHANGED_SYNC_FAST_PATH": "sometimes"}, wantErr: true},
		{name: "zero processes", values: map[string]string{"REMOTR_SERVER_PROCESSES": "0"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := fastPathConfigFromEnvironment(func(key string) string { return test.values[key] })
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil && (config.Enabled != test.wantEnabled || config.ServingProcesses != test.wantServing) {
				t.Fatalf("config = %+v, want enabled=%t serving=%d", config, test.wantEnabled, test.wantServing)
			}
		})
	}
}

func TestFastPathCheckpointIntervalBoundaries(t *testing.T) {
	tests := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{want: 5 * time.Minute},
		{value: "5m", want: 5 * time.Minute},
		{value: "7m30s", want: 7*time.Minute + 30*time.Second},
		{value: "10m", want: 10 * time.Minute},
		{value: "4m59s", wantErr: true},
		{value: "10m1s", wantErr: true},
		{value: "invalid", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			config, err := fastPathConfigFromEnvironment(func(key string) string {
				if key == "REMOTR_UNCHANGED_SYNC_CHECKPOINT_INTERVAL" {
					return test.value
				}
				return ""
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil && config.CheckpointInterval != test.want {
				t.Fatalf("interval = %s, want %s", config.CheckpointInterval, test.want)
			}
		})
	}
}

func TestFastPathResourceEnvironmentBoundaries(t *testing.T) {
	values := map[string]string{
		"REMOTR_UNCHANGED_SYNC_MAX_ENTRIES": "200",
		"REMOTR_UNCHANGED_SYNC_MAX_BYTES":   "1048576",
		"REMOTR_UNCHANGED_SYNC_TTL":         "8m",
	}
	config, err := fastPathConfigFromEnvironment(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxEntries != 200 || config.MaxBytes != 1048576 || config.TTL != 8*time.Minute {
		t.Fatalf("resource config = %+v", config)
	}
	values["REMOTR_UNCHANGED_SYNC_MAX_ENTRIES"] = "0"
	if _, err := fastPathConfigFromEnvironment(func(key string) string { return values[key] }); err == nil {
		t.Fatal("zero max entries accepted")
	}
}
