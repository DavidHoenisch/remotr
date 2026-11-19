// Package sudo manages validated, named sudoers.d fragments.
package sudo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	"github.com/DavidHoenisch/remotr/internal/applicators/filetx"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// Applicator stages a complete sudoers include tree before atomically
// activating the resource's one fragment.
type Applicator struct {
	Resource             models.SudoResource
	SudoersDir           string
	SudoersPath          string
	Owner                string
	Group                string
	Runner               executil.Runner
	LookupRecovery       func(string) error
	LookupRecoveryGroups func(string) ([]string, error)
	ValidateEffective    func(context.Context, string, string) error
	VerifyRecoveryAccess func(context.Context, string, string, string) error
	previous             []byte
	previousExists       bool
	rollbackArmed        bool
	rollback             *filetx.Handle
}

func (a *Applicator) ConfigureRollback(store *rollbackstore.Store, address, artifactDigest string) error {
	handle, err := filetx.New(store, address, artifactDigest, true)
	if err != nil {
		return err
	}
	a.rollback = handle
	return nil
}

func (a *Applicator) PreflightRollback(ctx context.Context) error {
	path, err := a.fragmentPath()
	if err != nil {
		return err
	}
	return a.rollback.Preflight(ctx, path)
}

// New creates an applicator. A runner may be supplied by the registry to
// retain an exact, injectable visudo argv boundary.
func New(resource models.SudoResource, runners ...executil.Runner) *Applicator {
	runner := executil.Runner(executil.SanitizedOSRunner{})
	if len(runners) > 0 && runners[0] != nil {
		runner = runners[0]
	}
	return &Applicator{
		Resource: resource, SudoersDir: "/etc/sudoers.d", SudoersPath: "/etc/sudoers", Owner: "root", Group: "root", Runner: runner,
		LookupRecovery: func(principal string) error {
			_, err := user.Lookup(principal)
			return err
		},
		LookupRecoveryGroups: defaultLookupUserGroups,
	}
}

func defaultLookupUserGroups(principal string) ([]string, error) {
	account, err := user.Lookup(principal)
	if err != nil {
		return nil, err
	}
	gids, err := account.GroupIds()
	if err != nil {
		return nil, err
	}
	groups := make([]string, 0, len(gids))
	for _, gid := range gids {
		if grp, grpErr := user.LookupGroupId(gid); grpErr == nil && grp.Name != "" {
			groups = append(groups, grp.Name)
		}
	}
	return groups, nil
}

func (a *Applicator) Name() string { return "sudo:" + a.Resource.Name }

func (a *Applicator) Description() string { return "sudo fragment " + a.Resource.Name }

func (a *Applicator) fragmentPath() (string, error) {
	if !filepath.IsAbs(a.SudoersDir) {
		return "", fmt.Errorf("sudoers directory must be absolute")
	}
	return filepath.Join(a.SudoersDir, a.Resource.FragmentFilename()), nil
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	path, err := a.fragmentPath()
	if err != nil {
		return nil, false
	}
	content, err := os.ReadFile(path) // #nosec G304 -- validated sudoers.d fragment path.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return nil, os.IsNotExist(err)
	}
	if err != nil {
		return nil, false
	}
	info, err := os.Lstat(path)
	if err != nil || !a.metadataCompliant(info) {
		return nil, false
	}
	return nil, string(content) == a.render()
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	_, compliant := a.State(ctx)
	if !compliant {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: "validated sudo fragment"}
	}
	res := executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: "validated sudo fragment"}
	if a.Resource.Lifecycle == models.LifecyclePresent {
		if a.Resource.AuditUnmanaged {
			unmanaged, total, truncated := a.detectUnmanagedFragments()
			if total > 0 {
				res.Status = executor.Drifted
				res.ReasonCode = "sudo_unmanaged_fragments"
				res.ObservedSummary = executor.RedactedSummary(fmt.Sprintf("%d unmanaged sudoers.d fragment(s) detected", total))
				res.Subresults = unmanaged
				res.SubresultsTruncated = truncated
				return res
			}
		}
		subresults, total, truncated := a.detectShadowingFragments()
		if total > 0 {
			res.ObservedSummary = executor.RedactedSummary(fmt.Sprintf("managed sudo fragment compliant; %d later fragment(s) may shadow managed subjects", total))
			res.Subresults = subresults
			res.SubresultsTruncated = truncated
		}
	}
	return res
}

func (a *Applicator) detectUnmanagedFragments() ([]executor.CheckSubresult, int, bool) {
	entries, err := os.ReadDir(a.SudoersDir)
	if err != nil {
		return nil, 0, false
	}
	allowed := map[string]struct{}{
		a.Resource.FragmentFilename(): {},
	}
	for _, name := range a.Resource.AllowedUnmanagedFragments {
		allowed[name] = struct{}{}
	}
	var subresults []executor.CheckSubresult
	total := 0
	truncated := false
	for _, entry := range entries {
		if !isActiveSudoersFragmentEntry(entry) {
			continue
		}
		if _, ok := allowed[entry.Name()]; ok {
			continue
		}
		total++
		if len(subresults) < executor.MaxCheckSubresults {
			subresults = append(subresults, executor.CheckSubresult{
				Target:          entry.Name(),
				Status:          executor.Drifted,
				ReasonCode:      "sudo_unmanaged_fragments",
				DesiredSummary:  "declared sudoers.d fragment",
				ObservedSummary: "unmanaged sudoers.d fragment present",
			})
		} else {
			truncated = true
		}
	}
	return subresults, total, truncated
}

func (a *Applicator) detectShadowingFragments() ([]executor.CheckSubresult, int, bool) {
	managedSubjects := map[string]struct{}{}
	for _, rule := range a.Resource.EffectiveRules() {
		for _, subj := range rule.Subjects {
			managedSubjects[subj] = struct{}{}
		}
	}
	if len(managedSubjects) == 0 {
		return nil, 0, false
	}
	entries, err := os.ReadDir(a.SudoersDir)
	if err != nil {
		return nil, 0, false
	}
	selfName := a.Resource.FragmentFilename()
	var subresults []executor.CheckSubresult
	total := 0
	truncated := false
	for _, entry := range entries {
		if !isActiveSudoersFragmentEntry(entry) || entry.Name() <= selfName {
			continue
		}
		content, err := os.ReadFile(filepath.Join(a.SudoersDir, entry.Name())) // #nosec G304 -- enumerated sudoers.d entry.
		if err != nil {
			continue
		}
		if !fragmentMatchesAnySubject(string(content), managedSubjects) {
			continue
		}
		total++
		if len(subresults) < executor.MaxCheckSubresults {
			subresults = append(subresults, executor.CheckSubresult{
				Target:          entry.Name(),
				Status:          executor.Drifted,
				ReasonCode:      "sudo_fragment_shadowed",
				DesiredSummary:  "managed sudo precedence",
				ObservedSummary: "later sudoers.d fragment matches managed subject",
			})
		} else {
			truncated = true
		}
	}
	return subresults, total, truncated
}

func fragmentMatchesAnySubject(content string, targets map[string]struct{}) bool {
	for _, line := range logicalSudoersLines(content) {
		subjects, ok := extractRuleSubjects(line)
		if !ok {
			continue
		}
		for _, subj := range subjects {
			if strings.HasPrefix(subj, "!") {
				continue
			}
			if subj == "ALL" {
				return true
			}
			if _, ok := targets[subj]; ok {
				return true
			}
		}
	}
	return false
}

// Preflight ensures all declared recovery identities are available and the
// staged effective sudoers configuration passes validation before a resource
// can modify access policy. Engine policy additionally leaves this access-risk
// resource report-only until enforce: true is declared.
func (a *Applicator) Preflight(ctx context.Context) error {
	if len(a.Resource.RecoveryPrincipals) == 0 {
		return fmt.Errorf("sudo %q has no recovery principal", a.Resource.Name)
	}
	lookup := a.LookupRecovery
	if lookup == nil {
		lookup = func(principal string) error { _, err := user.Lookup(principal); return err }
	}
	for _, principal := range a.Resource.RecoveryPrincipals {
		if err := lookup(principal); err != nil {
			return fmt.Errorf("recovery principal %q: %w", principal, err)
		}
	}
	return a.withStagedCandidate(ctx, a.Resource.Lifecycle == models.LifecyclePresent, a.render(), func(stageMain, stageDir string) error {
		for _, principal := range a.Resource.RecoveryPrincipals {
			if a.VerifyRecoveryAccess != nil {
				if err := a.VerifyRecoveryAccess(ctx, principal, stageMain, stageDir); err != nil {
					return errors.New("recovery principal failed effective sudo access verification")
				}
				continue
			}
			if err := a.verifyDefaultRecoveryAccess(principal, stageMain, stageDir); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *Applicator) verifyDefaultRecoveryAccess(principal, stageMain, stageDir string) error {
	if principal == "root" {
		return nil
	}
	var groups []string
	groupLookup := a.LookupRecoveryGroups
	if groupLookup == nil {
		groupLookup = defaultLookupUserGroups
	}
	if resolved, err := groupLookup(principal); err == nil {
		groups = resolved
	}
	hasGrant, err := stagedTreeHasGrantForPrincipal(stageMain, stageDir, principal, groups)
	if err != nil {
		return errors.New("staged effective sudoers recovery check failed")
	}
	if !hasGrant {
		return errors.New("recovery principal has no effective sudo grant in staged policy")
	}
	stdout, _, err := a.Runner.Run("sudo", "-l", "-U", principal)
	if err != nil {
		return errors.New("recovery principal has no verified effective sudo access")
	}
	outLower := strings.ToLower(string(stdout))
	if strings.Contains(outLower, "not allowed to run sudo") || !strings.Contains(outLower, "may run the following") {
		return errors.New("recovery principal has no verified effective sudo access")
	}
	return nil
}

func stagedTreeHasGrantForPrincipal(stageMain, stageDir, principal string, groups []string) (bool, error) {
	lines, err := collectStagedPolicyLines(stageMain, stageDir)
	if err != nil {
		return false, err
	}
	aliases := parseUserAliases(lines)
	matched := map[string]struct{}{
		principal: {},
		"ALL":     {},
	}
	for _, g := range groups {
		if g != "" {
			matched["%"+g] = struct{}{}
		}
	}
	// Resolve User_Alias definitions transitively (bounded iterations).
	for iter := 0; iter < 8; iter++ {
		changed := false
		for alias, members := range aliases {
			if _, ok := matched[alias]; ok {
				continue
			}
			for _, m := range members {
				if strings.HasPrefix(m, "!") {
					continue
				}
				if _, ok := matched[m]; ok {
					matched[alias] = struct{}{}
					changed = true
					break
				}
			}
		}
		if !changed {
			break
		}
	}
	for _, line := range lines {
		subjects, ok := extractRuleSubjects(line)
		if !ok {
			continue
		}
		for _, subj := range subjects {
			if strings.HasPrefix(subj, "!") {
				continue
			}
			if _, ok := matched[subj]; ok {
				return true, nil
			}
		}
	}
	return false, nil
}

func collectStagedPolicyLines(stageMain, stageDir string) ([]string, error) {
	var all []string
	mainBytes, err := os.ReadFile(stageMain) // #nosec G304 -- staged temp sudoers file.
	if err != nil {
		return nil, err
	}
	all = append(all, logicalSudoersLines(string(mainBytes))...)
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !isActiveSudoersFragmentEntry(entry) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(stageDir, entry.Name())) // #nosec G304 -- staged temp fragment.
		if err != nil {
			return nil, err
		}
		all = append(all, logicalSudoersLines(string(content))...)
	}
	return all, nil
}

func isActiveSudoersFragmentEntry(entry os.DirEntry) bool {
	if entry.IsDir() {
		return false
	}
	name := entry.Name()
	if name == "" || strings.HasPrefix(name, ".") || strings.Contains(name, ".") || strings.HasSuffix(name, "~") {
		return false
	}
	info, err := entry.Info()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return true
}

func logicalSudoersLines(content string) []string {
	rawLines := strings.Split(content, "\n")
	var out []string
	var cur strings.Builder
	for _, raw := range rawLines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "@") {
			if cur.Len() > 0 {
				out = append(out, strings.TrimSpace(cur.String()))
				cur.Reset()
			}
			continue
		}
		if strings.HasSuffix(trimmed, `\`) {
			cur.WriteString(strings.TrimSuffix(trimmed, `\`))
			cur.WriteByte(' ')
			continue
		}
		cur.WriteString(trimmed)
		out = append(out, strings.TrimSpace(cur.String()))
		cur.Reset()
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

func parseUserAliases(lines []string) map[string][]string {
	aliases := map[string][]string{}
	for _, line := range lines {
		if !strings.HasPrefix(line, "User_Alias") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "User_Alias"))
		for _, def := range strings.Split(rest, ":") {
			eq := strings.IndexByte(def, '=')
			if eq <= 0 {
				continue
			}
			aliasName := strings.TrimSpace(def[:eq])
			if aliasName == "" {
				continue
			}
			for _, member := range strings.Split(def[eq+1:], ",") {
				member = strings.TrimSpace(member)
				if member != "" {
					aliases[aliasName] = append(aliases[aliasName], member)
				}
			}
		}
	}
	return aliases
}

func extractRuleSubjects(line string) ([]string, bool) {
	if line == "" || strings.HasPrefix(line, "Defaults") ||
		strings.HasPrefix(line, "User_Alias") || strings.HasPrefix(line, "Runas_Alias") ||
		strings.HasPrefix(line, "Host_Alias") || strings.HasPrefix(line, "Cmnd_Alias") {
		return nil, false
	}
	if !strings.Contains(line, "=") {
		return nil, false
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return nil, false
	}
	parts := strings.Split(fields[0], ",")
	subjects := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			subjects = append(subjects, part)
		}
	}
	return subjects, len(subjects) > 0
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := a.Preflight(ctx); err != nil {
		return err
	}
	path, err := a.fragmentPath()
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path) // #nosec G304 -- validated sudoers.d fragment path.
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent && !exists {
		return appErr.ErrStateAlreadyMet
	}
	desired := a.render()
	if a.Resource.Lifecycle == models.LifecyclePresent && exists && string(current) == desired {
		info, statErr := os.Lstat(path)
		if statErr == nil && a.metadataCompliant(info) {
			return appErr.ErrStateAlreadyMet
		}
	}
	if a.rollback != nil {
		if err := a.rollback.Arm(ctx, path); err != nil {
			return err
		}
	} else {
		a.previous, a.previousExists, a.rollbackArmed = append([]byte(nil), current...), exists, true
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return os.Remove(path) // #nosec G703 -- validated sudoers.d fragment path.
	}
	return files.New(models.File{Name: a.Resource.FragmentFilename(), Path: path, Content: desired, Mode: []int{0o440}, Owner: a.Owner, Group: a.Group}).Apply(ctx)
}

func (a *Applicator) metadataCompliant(info os.FileInfo) bool {
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o440 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	owner, err := user.Lookup(a.Owner)
	if err != nil {
		return false
	}
	group, err := user.LookupGroup(a.Group)
	if err != nil {
		return false
	}
	uid, err := strconv.ParseUint(owner.Uid, 10, 32)
	if err != nil {
		return false
	}
	gid, err := strconv.ParseUint(group.Gid, 10, 32)
	return err == nil && stat.Uid == uint32(uid) && stat.Gid == uint32(gid)
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	rollbackClass := executor.RollbackNone
	if a.rollback != nil {
		rollbackClass = executor.RollbackTransactional
	}
	err := a.Apply(ctx)
	if err == nil {
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	}
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	}
	return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass, Err: err}
}

// Revert restores the protected transaction when registry configuration is
// present. Directly constructed providers retain local compatibility state,
// but ApplyResult deliberately advertises no rollback for that case.
func (a *Applicator) Revert(ctx context.Context) error {
	if a.rollback != nil {
		err := a.rollback.Rollback(ctx)
		if errors.Is(err, os.ErrNotExist) {
			return appErr.ErrNoOp
		}
		return err
	}
	path, err := a.fragmentPath()
	if err != nil {
		return err
	}
	if !a.rollbackArmed {
		return appErr.ErrNoOp
	}
	if !a.previousExists {
		return os.Remove(path) // #nosec G703 -- validated sudoers.d fragment path.
	}
	return files.New(models.File{Name: a.Resource.Name, Path: path, Content: string(a.previous), Mode: []int{0o440}}).Apply(ctx)
}

func (a *Applicator) render() string {
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return ""
	}
	var lines []string
	for _, d := range a.Resource.Defaults {
		lines = append(lines, renderSudoDefault(d))
	}
	for _, rule := range a.Resource.EffectiveRules() {
		lines = append(lines, renderSudoRule(rule))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderSudoDefault(d models.SudoDefault) string {
	header := "Defaults"
	switch d.Scope {
	case "user":
		header += ":" + d.Target
	case "runAs":
		header += ">" + d.Target
	case "command":
		header += "!" + d.Target
	}
	switch d.Option {
	case "logfile", "iolog_dir", "secure_path":
		return fmt.Sprintf(`%s %s="%s"`, header, d.Option, d.Value)
	case "env_keep", "env_delete":
		return fmt.Sprintf(`%s %s += "%s"`, header, d.Option, strings.Join(d.Values, " "))
	case "timestamp_timeout", "passwd_tries", "passwd_timeout", "umask", "lecture", "timestamp_type":
		return fmt.Sprintf("%s %s=%s", header, d.Option, d.Value)
	default:
		if d.Negate {
			return fmt.Sprintf("%s !%s", header, d.Option)
		}
		return fmt.Sprintf("%s %s", header, d.Option)
	}
}

func renderSudoRule(rule models.SudoRule) string {
	runAs := "ALL"
	if len(rule.RunAs) > 0 {
		runAs = strings.Join(rule.RunAs, ",")
	}
	prefix := strings.Join(rule.Subjects, ",") + " ALL=(" + runAs + ") "
	if len(rule.Tags) > 0 {
		prefix += strings.Join(rule.Tags, ": ") + ": "
	}
	cmds := make([]string, 0, len(rule.Commands)+len(rule.CommandSpecs))
	for _, cmd := range rule.Commands {
		cmds = append(cmds, strings.TrimSpace(cmd))
	}
	for _, spec := range rule.CommandSpecs {
		cmds = append(cmds, renderSudoCommandSpec(spec))
	}
	return prefix + strings.Join(cmds, ", ")
}

func renderSudoCommandSpec(spec models.SudoCommandSpec) string {
	var b strings.Builder
	if spec.Negate {
		b.WriteByte('!')
	}
	b.WriteString(spec.Path)
	if spec.NoArgs {
		b.WriteString(` ""`)
	} else if len(spec.Args) > 0 {
		b.WriteByte(' ')
		b.WriteString(strings.Join(spec.Args, " "))
	}
	return b.String()
}

func (a *Applicator) validateCandidate(ctx context.Context, present bool, content string) error {
	return a.withStagedCandidate(ctx, present, content, nil)
}

func (a *Applicator) withStagedCandidate(ctx context.Context, present bool, content string, afterVisudo func(stageMain, stageDir string) error) error {
	stageRoot, err := os.MkdirTemp("", "remotr-sudoers-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageRoot)
	stageDir := filepath.Join(stageRoot, "sudoers.d")
	if err := copyDir(a.SudoersDir, stageDir); err != nil {
		return err
	}
	mainBytes, err := os.ReadFile(a.SudoersPath) // #nosec G304 -- fixed root sudoers path.
	if err != nil {
		return err
	}
	mainText := string(mainBytes)
	hashInclude := "#includedir " + a.SudoersDir
	atInclude := "@includedir " + a.SudoersDir
	if !strings.Contains(mainText, hashInclude) && !strings.Contains(mainText, atInclude) {
		return fmt.Errorf("sudoers %q does not include managed fragment directory %q", a.SudoersPath, a.SudoersDir)
	}
	mainText = strings.ReplaceAll(mainText, hashInclude, "#includedir "+stageDir)
	mainText = strings.ReplaceAll(mainText, atInclude, "@includedir "+stageDir)
	stageMain := filepath.Join(stageRoot, "sudoers")
	if err := os.WriteFile(stageMain, []byte(mainText), 0o440); err != nil {
		return err
	}
	stageFragment := filepath.Join(stageDir, a.Resource.FragmentFilename())
	if present {
		if err := os.Remove(stageFragment); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(stageFragment, []byte(content), 0o440); err != nil {
			return err
		}
	} else if err := os.Remove(stageFragment); err != nil && !os.IsNotExist(err) {
		return err
	}
	if a.ValidateEffective != nil {
		if err := a.ValidateEffective(ctx, stageMain, stageDir); err != nil {
			return errors.New("staged effective sudoers validation failed")
		}
	} else {
		_, _, err = a.Runner.Run("visudo", "-cf", stageMain)
		if err != nil {
			return errors.New("visudo rejected staged effective sudoers")
		}
	}
	if afterVisudo != nil {
		return afterVisudo(stageMain, stageDir)
	}
	return nil
}

func copyDir(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		sourcePath := filepath.Join(source, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("sudoers fragment %q is not a regular file", sourcePath)
		}
		content, err := os.ReadFile(sourcePath) // #nosec G304 -- enumerated root-owned sudoers.d entry.
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), content, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}
