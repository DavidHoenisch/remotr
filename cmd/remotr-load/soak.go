package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/performance"
)

type composeGrowthProbe struct {
	composeFile        string
	diagnosticsService string
	agentServices      []string
}

func (p composeGrowthProbe) Snapshot(ctx context.Context) (performance.GrowthSample, error) {
	rawMetrics, err := p.composeOutput(ctx, p.diagnosticsService, "wget", "-qO-", "http://127.0.0.1:6060/debug/remotr/metrics")
	if err != nil {
		return performance.GrowthSample{}, fmt.Errorf("read server runtime metrics: %w", err)
	}
	var runtimeMetrics performance.RuntimeMetrics
	if err := json.Unmarshal(rawMetrics, &runtimeMetrics); err != nil {
		return performance.GrowthSample{}, fmt.Errorf("decode server runtime metrics: %w", err)
	}
	processStat, err := p.composeOutput(ctx, p.diagnosticsService, "cat", "/proc/1/stat")
	if err != nil {
		return performance.GrowthSample{}, fmt.Errorf("read server process CPU: %w", err)
	}
	serverCPUJiffies, err := parseProcessCPUJiffies(processStat)
	if err != nil {
		return performance.GrowthSample{}, fmt.Errorf("parse server process CPU: %w", err)
	}

	var agentRSS, temporaryBytes, rollbackBytes int64
	var agentGoroutines int64
	for _, service := range p.agentServices {
		agentMetricsRaw, err := p.composeOutput(ctx, service, "wget", "-qO-", "http://127.0.0.1:6060/debug/remotr/metrics")
		if err != nil {
			return performance.GrowthSample{}, fmt.Errorf("read %s runtime metrics: %w", service, err)
		}
		var agentMetrics performance.RuntimeMetrics
		if err := json.Unmarshal(agentMetricsRaw, &agentMetrics); err != nil {
			return performance.GrowthSample{}, fmt.Errorf("decode %s runtime metrics: %w", service, err)
		}
		agentGoroutines += int64(agentMetrics.Goroutines)
		status, err := p.composeOutput(ctx, service, "cat", "/proc/1/status")
		if err != nil {
			return performance.GrowthSample{}, fmt.Errorf("read %s process status: %w", service, err)
		}
		rss, err := parseStatusRSS(status)
		if err != nil {
			return performance.GrowthSample{}, fmt.Errorf("read %s RSS: %w", service, err)
		}
		agentRSS += rss
		usage, err := p.composeOutput(ctx, service, "sh", "-c", agentStateUsageCommand)
		if err != nil {
			return performance.GrowthSample{}, fmt.Errorf("read %s state usage: %w", service, err)
		}
		temporary, rollback, err := parseAgentStateUsage(usage)
		if err != nil {
			return performance.GrowthSample{}, fmt.Errorf("parse %s state usage: %w", service, err)
		}
		temporaryBytes += temporary
		rollbackBytes += rollback
	}
	return performance.GrowthSample{
		ServerHeapBytes: int64(runtimeMetrics.HeapAllocBytes), ServerGoroutines: int64(runtimeMetrics.Goroutines),
		ServerCPUJiffies: serverCPUJiffies,
		AgentRSSBytes:    agentRSS, AgentGoroutines: agentGoroutines,
		TemporaryBytes: temporaryBytes, RollbackBytes: rollbackBytes,
	}, nil
}

const agentStateUsageCommand = `size() { total=0; for path in "$@"; do if [ -e "$path" ]; then bytes=$(du -sb "$path" | awk '{print $1}'); total=$((total + bytes)); fi; done; printf '%s' "$total"; }; printf 'temporary=%s\n' "$(size /var/lib/remotr/tmp /var/lib/remotr/temp /var/lib/remotr/temporary)"; printf 'rollback=%s\n' "$(size /var/lib/remotr/rollback /var/lib/remotr/rollbacks /var/lib/remotr/network-transactions)"`

func parseAgentStateUsage(output []byte) (temporaryBytes, rollbackBytes int64, err error) {
	seen := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		key, raw, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok || (key != "temporary" && key != "rollback") {
			return 0, 0, fmt.Errorf("unexpected state usage line %q", scanner.Text())
		}
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value < 0 {
			return 0, 0, fmt.Errorf("invalid %s bytes %q", key, raw)
		}
		seen[key] = true
		if key == "temporary" {
			temporaryBytes = value
		} else {
			rollbackBytes = value
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if !seen["temporary"] || !seen["rollback"] {
		return 0, 0, fmt.Errorf("state usage omitted temporary or rollback bytes")
	}
	return temporaryBytes, rollbackBytes, nil
}

func (p composeGrowthProbe) composeOutput(ctx context.Context, service string, command ...string) ([]byte, error) {
	args := []string{"compose", "-f", p.composeFile, "exec", "-T", service}
	args = append(args, command...)
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker compose exec %s: %w: %s", service, err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func parseStatusRSS(status []byte) (int64, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(status)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || fields[0] != "VmRSS:" || fields[2] != "kB" {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		return value * 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("VmRSS not found")
}

func parseProcessCPUJiffies(stat []byte) (int64, error) {
	closing := strings.LastIndexByte(strings.TrimSpace(string(stat)), ')')
	if closing < 0 {
		return 0, fmt.Errorf("process stat omitted command terminator")
	}
	fields := strings.Fields(strings.TrimSpace(string(stat))[closing+1:])
	if len(fields) <= 12 {
		return 0, fmt.Errorf("process stat omitted CPU fields")
	}
	user, err := strconv.ParseInt(fields[11], 10, 64)
	if err != nil || user < 0 {
		return 0, fmt.Errorf("invalid process user CPU jiffies")
	}
	system, err := strconv.ParseInt(fields[12], 10, 64)
	if err != nil || system < 0 {
		return 0, fmt.Errorf("invalid process system CPU jiffies")
	}
	return user + system, nil
}

func loadBudgets(path string) (performance.BudgetFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return performance.BudgetFile{}, err
	}
	budgets, err := performance.ParseBudgets(data)
	if err != nil {
		return performance.BudgetFile{}, err
	}
	return budgets, nil
}
