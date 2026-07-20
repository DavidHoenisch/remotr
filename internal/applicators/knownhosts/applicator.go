// Package knownhosts manages structured, marker-owned OpenSSH known_hosts
// entries while preserving unrelated host trust records.
package knownhosts

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- OpenSSH known_hosts hash format requires HMAC-SHA1.
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

type Applicator struct {
	Resource      models.KnownHostResource
	SystemPath    string
	LookupUser    func(string) (interactiveuser.Account, error)
	Random        io.Reader
	rollbackStore *rollbackstore.Store
	rollbackAddr  string
	rollbackHash  string
}

func New(resource models.KnownHostResource) *Applicator {
	return &Applicator{Resource: resource, SystemPath: "/etc/ssh/ssh_known_hosts", Random: rand.Reader}
}

func (a *Applicator) Name() string { return "knownHost:" + a.Resource.Name }

func (a *Applicator) Description() string { return "known host " + a.Resource.Name }

// ConfigureRollback binds host-trust state to protected restart-safe file
// recovery. User-scoped targets retain descriptor-safe no-follow traversal.
func (a *Applicator) ConfigureRollback(store *rollbackstore.Store, address, artifactDigest string) error {
	if _, err := rollbackstore.NewHandle(store, address, artifactDigest, true); err != nil {
		return err
	}
	a.rollbackStore, a.rollbackAddr, a.rollbackHash = store, address, artifactDigest
	return nil
}

func (a *Applicator) account() (interactiveuser.Account, error) {
	if a.LookupUser != nil {
		return a.LookupUser(a.Resource.User)
	}
	u, err := user.Lookup(a.Resource.User)
	if err != nil {
		return interactiveuser.Account{}, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return interactiveuser.Account{}, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return interactiveuser.Account{}, err
	}
	return interactiveuser.Account{Username: u.Username, UID: uid, GID: gid, HomeDir: u.HomeDir}, nil
}

func (a *Applicator) target() (string, *interactiveuser.Account, error) {
	if a.Resource.Scope == models.KnownHostScopeSystem {
		if !filepath.IsAbs(a.SystemPath) {
			return "", nil, fmt.Errorf("system known_hosts path must be absolute")
		}
		return a.SystemPath, nil, nil
	}
	account, err := a.account()
	if err != nil {
		return "", nil, err
	}
	path, err := interactiveuser.HomePath(account.HomeDir, filepath.Join(".ssh", "known_hosts"))
	if err != nil {
		return "", nil, err
	}
	return path, &account, nil
}

func (a *Applicator) read(path string, account *interactiveuser.Account) ([]byte, bool, error) {
	if account != nil {
		return files.ReadOwnedUnder(account.HomeDir, path)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed root-owned system path or controlled test seam.
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	path, account, err := a.target()
	if err != nil {
		return nil, false
	}
	data, _, err := a.read(path, account)
	if err != nil {
		return nil, false
	}
	before, block, after, found, err := splitBlock(string(data), a.Resource.Name)
	if err != nil {
		return nil, false
	}
	_ = before
	_ = after
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return nil, !found
	}
	if !found {
		return nil, false
	}
	return nil, a.blockCompliant(block)
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	_, compliant := a.State(ctx)
	if compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: "managed known-host entry"}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: "managed known-host entry"}
}

func (a *Applicator) Apply(ctx context.Context) error {
	if _, compliant := a.State(ctx); compliant {
		return appErr.ErrStateAlreadyMet
	}
	path, account, err := a.target()
	if err != nil {
		return err
	}
	data, _, err := a.read(path, account)
	if err != nil {
		return err
	}
	before, _, after, _, err := splitBlock(string(data), a.Resource.Name)
	if err != nil {
		return err
	}
	if a.Resource.Lifecycle == models.LifecyclePresent {
		var conflicts []string
		before, conflicts = a.removeOrFindConflicts(before)
		var afterConflicts []string
		after, afterConflicts = a.removeOrFindConflicts(after)
		conflicts = append(conflicts, afterConflicts...)
		if len(conflicts) > 0 && !a.Resource.ReplaceExisting {
			return fmt.Errorf("knownHost %q conflicts with existing host key; set replaceExisting to replace it", a.Resource.Name)
		}
	}
	lines := append([]string(nil), before...)
	if a.Resource.Lifecycle == models.LifecyclePresent {
		entries, err := a.renderEntries()
		if err != nil {
			return err
		}
		lines = append(lines, "# >>> remotr known_hosts "+a.Resource.Name+" >>>")
		lines = append(lines, entries...)
		lines = append(lines, "# <<< remotr known_hosts "+a.Resource.Name+" <<<")
	}
	lines = append(lines, after...)
	desired := "\n"
	if len(lines) > 0 {
		desired = strings.Join(lines, "\n") + "\n"
	}
	if string(data) == desired {
		return appErr.ErrStateAlreadyMet
	}
	file := models.File{Name: a.Resource.Name, Path: path, Content: desired, Mode: []int{0o644}}
	provider, err := a.fileProvider(file, account)
	if err != nil {
		return err
	}
	return provider.Apply(ctx)
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	rollbackClass := executor.RollbackNone
	if a.rollbackStore != nil {
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

func (a *Applicator) PreflightRollback(ctx context.Context) error {
	path, account, err := a.target()
	if err != nil {
		return err
	}
	provider, err := a.fileProvider(models.File{Name: a.Resource.Name, Path: path}, account)
	if err != nil {
		return err
	}
	return provider.PreflightRollback(ctx)
}

func (a *Applicator) Revert(ctx context.Context) error {
	path, account, err := a.target()
	if err != nil {
		return err
	}
	provider, err := a.fileProvider(models.File{Name: a.Resource.Name, Path: path}, account)
	if err != nil {
		return err
	}
	return provider.Revert(ctx)
}

func (a *Applicator) fileProvider(file models.File, account *interactiveuser.Account) (*files.Applicator, error) {
	var provider *files.Applicator
	if account != nil {
		file.Mode = []int{0o600}
		provider = files.NewOwnedUnder(file, account.HomeDir, account.UID, account.GID)
	} else {
		provider = files.New(file)
	}
	if a.rollbackStore != nil {
		if err := provider.ConfigureSensitiveRollback(a.rollbackStore, a.rollbackAddr, a.rollbackHash); err != nil {
			return nil, err
		}
	}
	return provider, nil
}

func splitBlock(content, name string) (before, block, after []string, found bool, err error) {
	start := "# >>> remotr known_hosts " + name + " >>>"
	end := "# <<< remotr known_hosts " + name + " <<<"
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if content == "" {
		lines = nil
	}
	startAt, endAt := -1, -1
	for i, line := range lines {
		if line == start {
			if startAt >= 0 {
				return nil, nil, nil, false, fmt.Errorf("duplicate known-host marker %q", name)
			}
			startAt = i
		}
		if line == end {
			if startAt < 0 || endAt >= 0 {
				return nil, nil, nil, false, fmt.Errorf("malformed known-host marker %q", name)
			}
			endAt = i
		}
	}
	if (startAt < 0) != (endAt < 0) {
		return nil, nil, nil, false, fmt.Errorf("malformed known-host marker %q", name)
	}
	if startAt < 0 {
		return lines, nil, nil, false, nil
	}
	return lines[:startAt], lines[startAt+1 : endAt], lines[endAt+1:], true, nil
}

func (a *Applicator) blockCompliant(block []string) bool {
	if a.Resource.Hashing == models.KnownHostHashPlain {
		entries, err := a.plainEntries()
		return err == nil && strings.Join(block, "\n") == strings.Join(entries, "\n")
	}
	if len(block) != len(a.Resource.Hosts) {
		return false
	}
	matched := make(map[string]bool, len(a.Resource.Hosts))
	for _, line := range block {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[1] != a.Resource.Type || fields[2] != a.Resource.Key || strings.Join(fields[3:], " ") != a.Resource.Comment {
			return false
		}
		matchedHost := ""
		for _, host := range a.Resource.Hosts {
			if !matched[host] && hashedHostMatches(fields[0], host) {
				matchedHost = host
				break
			}
		}
		if matchedHost == "" {
			return false
		}
		matched[matchedHost] = true
	}
	return len(matched) == len(a.Resource.Hosts)
}

func (a *Applicator) removeOrFindConflicts(lines []string) ([]string, []string) {
	out := make([]string, 0, len(lines))
	var conflicts []string
	for _, line := range lines {
		if a.conflicts(line) {
			conflicts = append(conflicts, line)
			if a.Resource.ReplaceExisting {
				continue
			}
		}
		out = append(out, line)
	}
	return out, conflicts
}

func (a *Applicator) conflicts(line string) bool {
	if strings.HasPrefix(line, "#") {
		return false
	}
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[1] != a.Resource.Type || fields[2] == a.Resource.Key {
		return false
	}
	for _, actual := range strings.Split(fields[0], ",") {
		for _, wanted := range a.Resource.Hosts {
			if actual == wanted || hashedHostMatches(actual, wanted) {
				return true
			}
		}
	}
	return false
}

func (a *Applicator) renderEntries() ([]string, error) {
	if a.Resource.Hashing == models.KnownHostHashPlain {
		return a.plainEntries()
	}
	entries := make([]string, 0, len(a.Resource.Hosts))
	for _, host := range a.Resource.Hosts {
		hashed, err := a.hashHost(host)
		if err != nil {
			return nil, err
		}
		entries = append(entries, a.render(hashed))
	}
	return entries, nil
}

func (a *Applicator) plainEntries() ([]string, error) {
	return []string{a.render(strings.Join(a.Resource.Hosts, ","))}, nil
}

func (a *Applicator) render(hosts string) string {
	line := hosts + " " + a.Resource.Type + " " + a.Resource.Key
	if a.Resource.Comment != "" {
		line += " " + a.Resource.Comment
	}
	return line
}

func (a *Applicator) hashHost(host string) (string, error) {
	random := a.Random
	if random == nil {
		random = rand.Reader
	}
	salt := make([]byte, sha1.Size)
	if _, err := io.ReadFull(random, salt); err != nil {
		return "", err
	}
	mac := hmac.New(sha1.New, salt) // #nosec G401 -- OpenSSH known_hosts hash format requires HMAC-SHA1.
	_, _ = mac.Write([]byte(host))
	return "|1|" + base64.StdEncoding.EncodeToString(salt) + "|" + base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

func hashedHostMatches(value, host string) bool {
	parts := strings.Split(value, "|")
	if len(parts) != 4 || parts[0] != "" || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	mac := hmac.New(sha1.New, salt) // #nosec G401 -- OpenSSH known_hosts hash format requires HMAC-SHA1.
	_, _ = mac.Write([]byte(host))
	return hmac.Equal(want, mac.Sum(nil))
}
