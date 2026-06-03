package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/applicators/agentinstall"
	"github.com/DavidHoenisch/remotr/internal/applicators/bootstrap"
	"github.com/DavidHoenisch/remotr/internal/applicators/command"
	"github.com/DavidHoenisch/remotr/internal/applicators/downloads"
	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	pkgfactory "github.com/DavidHoenisch/remotr/internal/applicators/packages"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemd"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemduser"
	"github.com/DavidHoenisch/remotr/internal/applicators/userfiles"
	"github.com/DavidHoenisch/remotr/internal/applicators/users"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Policy controls whether drift triggers apply.
type Policy string

const (
	PolicyAuto   Policy = "auto"
	PolicyReport Policy = "report"
)

type Kind int

const (
	KindPackage Kind = iota
	KindFile
	KindDownload
	KindFileCritical
	KindUser
	KindUserFile
	KindSystemd
	KindSystemdUser
	KindBootstrap
	KindAgentInstall
	KindCommand
)

type node struct {
	Address            string
	ConfigName         string
	Name               string
	Kind               Kind
	Handler            executor.Handler
	DependsOn          []string
	PreApplyValidation []string
}

// DriftItem describes one resource out of compliance.
type DriftItem struct {
	Address     string
	Name        string
	Description string
}

// DriftReport summarizes check results.
type DriftReport struct {
	Items        []DriftItem
	InCompliance bool
}

// ApplyResult summarizes an apply run.
type ApplyResult struct {
	Applied []string
	Skipped []string
	Failed  *ApplyFailure
}

type ApplyFailure struct {
	Address string
	Err     error
}

// Engine runs check/apply over resolved desired state.
type Engine struct {
	nodes    []node
	exec     executil.Runner
	executor *executor.Applicator
}

// New builds an engine from resolved state.
func New(resolved resolve.ResolvedState, f facts.Facts, exec executil.Runner) (*Engine, error) {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	e := &Engine{exec: exec, executor: executor.New()}
	nodes, err := buildNodes(resolved, f, exec)
	if err != nil {
		return nil, err
	}
	order, err := sortNodes(nodes)
	if err != nil {
		return nil, err
	}
	e.nodes = order
	return e, nil
}

func buildNodes(resolved resolve.ResolvedState, f facts.Facts, exec executil.Runner) ([]node, error) {
	var nodes []node
	addresses := map[string]struct{}{}

	add := func(n node) {
		nodes = append(nodes, n)
		addresses[n.Address] = struct{}{}
	}

	for _, cfg := range resolved.Configurations {
		for _, pkg := range cfg.Packages {
			h, err := pkgfactory.SelectPackageApplicator(f.Distro, pkg, exec)
			if err != nil {
				return nil, err
			}
			add(node{
				Address:            models.ResourceAddress(cfg.Name, pkg.Name),
				ConfigName:         cfg.Name,
				Name:               pkg.Name,
				Kind:               KindPackage,
				Handler:            h,
				DependsOn:          append([]string(nil), pkg.DependsOn...),
				PreApplyValidation: append([]string(nil), pkg.PreApplyValidation...),
			})
		}
		for _, file := range cfg.Files {
			kind := KindFile
			if isCriticalFile(file) {
				kind = KindFileCritical
			}
			add(node{
				Address:            models.ResourceAddress(cfg.Name, file.Name),
				ConfigName:         cfg.Name,
				Name:               file.Name,
				Kind:               kind,
				Handler:            files.New(file),
				DependsOn:          append([]string(nil), file.DependsOn...),
				PreApplyValidation: append([]string(nil), file.PreApplyValidation...),
			})
		}
		for _, dl := range cfg.Downloads {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, dl.Name),
				ConfigName:         cfg.Name,
				Name:               dl.Name,
				Kind:               KindDownload,
				Handler:            downloads.New(dl, exec),
				DependsOn:          append([]string(nil), dl.DependsOn...),
				PreApplyValidation: append([]string(nil), dl.PreApplyValidation...),
			})
		}
		for _, u := range cfg.Users {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, u.Name),
				ConfigName:         cfg.Name,
				Name:               u.Name,
				Kind:               KindUser,
				Handler:            users.New(u),
				DependsOn:          append([]string(nil), u.DependsOn...),
				PreApplyValidation: append([]string(nil), u.PreApplyValidation...),
			})
		}
		for _, uf := range cfg.UserFiles {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, uf.Name),
				ConfigName:         cfg.Name,
				Name:               uf.Name,
				Kind:               KindUserFile,
				Handler:            userfiles.New(uf),
				DependsOn:          append([]string(nil), uf.DependsOn...),
				PreApplyValidation: append([]string(nil), uf.PreApplyValidation...),
			})
		}
		for _, s := range cfg.Systemd {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, s.Name),
				ConfigName:         cfg.Name,
				Name:               s.Name,
				Kind:               KindSystemd,
				Handler:            systemd.New(s, exec),
				DependsOn:          append([]string(nil), s.DependsOn...),
				PreApplyValidation: append([]string(nil), s.PreApplyValidation...),
			})
		}
		for _, su := range cfg.SystemdUser {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, su.Name),
				ConfigName:         cfg.Name,
				Name:               su.Name,
				Kind:               KindSystemdUser,
				Handler:            systemduser.New(su, exec),
				DependsOn:          append([]string(nil), su.DependsOn...),
				PreApplyValidation: append([]string(nil), su.PreApplyValidation...),
			})
		}
		for _, b := range cfg.Bootstrap {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, b.Name),
				ConfigName:         cfg.Name,
				Name:               b.Name,
				Kind:               KindBootstrap,
				Handler:            bootstrap.New(b, exec),
				DependsOn:          append([]string(nil), b.DependsOn...),
				PreApplyValidation: append([]string(nil), b.PreApplyValidation...),
			})
		}
		for _, ag := range cfg.AgentInstall {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, ag.Name),
				ConfigName:         cfg.Name,
				Name:               ag.Name,
				Kind:               KindAgentInstall,
				Handler:            agentinstall.New(ag, exec),
				DependsOn:          append([]string(nil), ag.DependsOn...),
				PreApplyValidation: append([]string(nil), ag.PreApplyValidation...),
			})
		}
		for _, c := range cfg.Commands {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, c.Name),
				ConfigName:         cfg.Name,
				Name:               c.Name,
				Kind:               KindCommand,
				Handler:            command.New(c, exec),
				DependsOn:          append([]string(nil), c.DependsOn...),
				PreApplyValidation: append([]string(nil), c.PreApplyValidation...),
			})
		}
	}

	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			if _, ok := addresses[dep]; !ok {
				return nil, fmt.Errorf("unknown dependency %q for resource %q", dep, n.Address)
			}
		}
	}
	return nodes, nil
}

func isCriticalFile(f models.File) bool {
	if len(f.PreApplyValidation) > 0 {
		return true
	}
	return strings.HasPrefix(f.Path, "/etc/ssh")
}

func defaultTier(k Kind) int {
	switch k {
	case KindPackage:
		return 0
	case KindFile:
		return 1
	case KindDownload:
		return 2
	case KindFileCritical:
		return 3
	case KindUser:
		return 4
	case KindUserFile:
		return 5
	case KindSystemd:
		return 6
	case KindSystemdUser:
		return 7
	case KindBootstrap:
		return 8
	case KindAgentInstall:
		return 9
	case KindCommand:
		return 10
	default:
		return 99
	}
}

// sortNodes orders nodes by depends_on (topological) with default tier tiebreaker.
func sortNodes(nodes []node) ([]node, error) {
	byAddr := make(map[string]node, len(nodes))
	inDegree := make(map[string]int, len(nodes))
	adj := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		byAddr[n.Address] = n
		inDegree[n.Address] = 0
	}
	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			adj[dep] = append(adj[dep], n.Address)
			inDegree[n.Address]++
		}
	}

	var queue []string
	for addr, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, addr)
		}
	}
	sortQueue(queue, byAddr)

	var order []node
	for len(queue) > 0 {
		addr := queue[0]
		queue = queue[1:]
		order = append(order, byAddr[addr])
		for _, next := range adj[addr] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
		sortQueue(queue, byAddr)
	}
	if len(order) != len(nodes) {
		return nil, fmt.Errorf("dependency cycle detected")
	}
	return order, nil
}

func sortQueue(addrs []string, byAddr map[string]node) {
	for i := 0; i < len(addrs); i++ {
		for j := i + 1; j < len(addrs); j++ {
			a, b := byAddr[addrs[i]], byAddr[addrs[j]]
			ta, tb := defaultTier(a.Kind), defaultTier(b.Kind)
			if tb < ta || (tb == ta && addrs[j] < addrs[i]) {
				addrs[i], addrs[j] = addrs[j], addrs[i]
			}
		}
	}
}

// CheckAll returns drift for all resources.
func (e *Engine) CheckAll(ctx context.Context) DriftReport {
	var items []DriftItem
	for _, n := range e.nodes {
		_, met := n.Handler.State(ctx)
		if !met {
			items = append(items, DriftItem{
				Address:     n.Address,
				Name:        n.Name,
				Description: n.Handler.Description(),
			})
		}
	}
	return DriftReport{Items: items, InCompliance: len(items) == 0}
}

// ApplyAll applies drifted resources in order when policy is auto.
func (e *Engine) ApplyAll(ctx context.Context, policy Policy) ApplyResult {
	report := e.CheckAll(ctx)
	result := ApplyResult{}
	if policy == PolicyReport {
		for _, item := range report.Items {
			result.Skipped = append(result.Skipped, item.Address)
		}
		return result
	}
	for _, n := range e.nodes {
		_, met := n.Handler.State(ctx)
		if met {
			continue
		}
		if err := e.runPreApplyValidation(n); err != nil {
			result.Failed = &ApplyFailure{Address: n.Address, Err: err}
			return result
		}
		if err := e.executor.ApplyState(ctx, n.Handler); err != nil {
			result.Failed = &ApplyFailure{Address: n.Address, Err: err}
			return result
		}
		result.Applied = append(result.Applied, n.Address)
	}
	return result
}

func (e *Engine) runPreApplyValidation(n node) error {
	for _, cmdline := range n.PreApplyValidation {
		parts := strings.Fields(cmdline)
		if len(parts) == 0 {
			continue
		}
		_, _, err := e.exec.Run(parts[0], parts[1:]...)
		if err != nil {
			return fmt.Errorf("pre-apply validation %q for %s: %w", cmdline, n.Address, err)
		}
	}
	return nil
}

// NodeCount returns the number of resources (for tests).
func (e *Engine) NodeCount() int { return len(e.nodes) }

// NodeOrder returns resource addresses in apply order (for tests).
func (e *Engine) NodeOrder() []string {
	out := make([]string, len(e.nodes))
	for i, n := range e.nodes {
		out[i] = n.Address
	}
	return out
}
