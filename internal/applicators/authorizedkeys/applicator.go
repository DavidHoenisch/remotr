// Package authorizedkeys manages structured, marked OpenSSH authorized_keys
// sets without treating the file as an unowned text blob.
package authorizedkeys

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// Applicator applies one resource-owned marked set in a user's
// ~/.ssh/authorized_keys file.
type Applicator struct {
	Resource      models.AuthorizedKeyResource
	LookupUser    func(string) (interactiveuser.Account, error)
	RecoveryCheck func(string) error
	rollbackStore *rollbackstore.Store
	rollbackAddr  string
	rollbackHash  string
}

func New(resource models.AuthorizedKeyResource) *Applicator {
	return &Applicator{Resource: resource}
}

func (a *Applicator) Name() string { return "authorizedKey:" + a.Resource.Name }

func (a *Applicator) Description() string {
	return fmt.Sprintf("authorized keys for %s", a.Resource.User)
}

// ConfigureRollback binds this access provider to protected restart-safe file
// recovery without exposing the user-writable path to generic path traversal.
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
	return accountFromUser(u)
}

func accountFromUser(u *user.User) (interactiveuser.Account, error) {
	var account interactiveuser.Account
	if _, err := fmt.Sscanf(u.Uid, "%d", &account.UID); err != nil {
		return account, fmt.Errorf("parse uid for %s: %w", u.Username, err)
	}
	if _, err := fmt.Sscanf(u.Gid, "%d", &account.GID); err != nil {
		return account, fmt.Errorf("parse gid for %s: %w", u.Username, err)
	}
	account.Username, account.HomeDir = u.Username, u.HomeDir
	return account, nil
}

func (a *Applicator) path(account interactiveuser.Account) (string, error) {
	return interactiveuser.HomePath(account.HomeDir, filepath.Join(".ssh", "authorized_keys"))
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	account, err := a.account()
	if err != nil {
		return nil, false
	}
	path, err := a.path(account)
	if err != nil {
		return nil, false
	}
	content, exists, err := files.ReadOwnedUnder(account.HomeDir, path)
	if err != nil {
		return nil, false
	}
	if !exists {
		content = nil
	}
	desired, err := a.desired(string(content))
	if err != nil {
		return nil, false
	}
	return nil, contentMatches(string(content), desired)
}

// Check exposes a redacted structured observation without publishing public
// key material in generic drift output.
func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	_, compliant := a.State(ctx)
	if compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: "managed authorized key set"}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: "managed authorized key set"}
}

func (a *Applicator) Apply(ctx context.Context) error {
	account, err := a.account()
	if err != nil {
		return err
	}
	path, err := a.path(account)
	if err != nil {
		return err
	}
	content, exists, err := files.ReadOwnedUnder(account.HomeDir, path)
	if err != nil {
		return err
	}
	if !exists {
		content = nil
	}
	desired, err := a.desired(string(content))
	if err != nil {
		return err
	}
	if contentMatches(string(content), desired) {
		return appErr.ErrStateAlreadyMet
	}
	writeContent := desired
	if writeContent == "" {
		// The legacy file handler interprets an empty content field as
		// metadata-only. A blank line makes removal of a sole owned block
		// an explicit atomic content transition while remaining valid SSH input.
		writeContent = "\n"
	}
	file := models.File{Name: a.Resource.Name, Path: path, Content: writeContent, Mode: []int{0o600}}
	provider, err := a.fileProvider(file, account)
	if err != nil {
		return err
	}
	if err := provider.Apply(ctx); err != nil {
		return err
	}
	return nil
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

// Preflight proves a declared recovery identity is still resolvable before an
// authoritative SSH set can be enforced as an access-risk resource.
func (a *Applicator) Preflight(_ context.Context) error {
	if a.Resource.Ownership != models.OwnershipAuthoritative {
		return nil
	}
	if len(a.Resource.RecoveryPrincipals) == 0 {
		return fmt.Errorf("authoritative authorizedKey %q has no recovery principal", a.Resource.Name)
	}
	check := a.RecoveryCheck
	if check == nil {
		check = func(principal string) error { _, err := user.Lookup(principal); return err }
	}
	for _, principal := range a.Resource.RecoveryPrincipals {
		if err := check(principal); err != nil {
			return fmt.Errorf("recovery principal %q: %w", principal, err)
		}
	}
	return nil
}

func (a *Applicator) Revert(ctx context.Context) error {
	account, err := a.account()
	if err != nil {
		return err
	}
	path, err := a.path(account)
	if err != nil {
		return err
	}
	provider, err := a.fileProvider(models.File{Name: a.Resource.Name, Path: path}, account)
	if err != nil {
		return err
	}
	return provider.Revert(ctx)
}

func (a *Applicator) fileProvider(file models.File, account interactiveuser.Account) (*files.Applicator, error) {
	provider := files.NewOwnedUnder(file, account.HomeDir, account.UID, account.GID)
	if a.rollbackStore != nil {
		if err := provider.ConfigureSensitiveRollback(a.rollbackStore, a.rollbackAddr, a.rollbackHash); err != nil {
			return nil, err
		}
	}
	return provider, nil
}

func (a *Applicator) desired(existing string) (string, error) {
	entries := make([]string, 0, len(a.Resource.Entries))
	if a.Resource.Lifecycle == models.LifecyclePresent {
		for _, entry := range a.Resource.Entries {
			entries = append(entries, render(entry))
		}
	}
	return replaceMarkedBlock(existing, a.Resource.Name, entries, a.Resource.Lifecycle, a.Resource.Ownership)
}

func render(entry models.AuthorizedKeyEntry) string {
	restrictions := append([]string(nil), entry.Restrictions...)
	if len(entry.Principals) > 0 {
		restrictions = append(restrictions, `principals="`+strings.Join(entry.Principals, ",")+`"`)
	}
	prefix := ""
	if len(restrictions) > 0 {
		prefix = strings.Join(restrictions, ",") + " "
	}
	line := prefix + entry.Type + " " + entry.Key
	if entry.Comment != "" {
		line += " " + entry.Comment
	}
	return line
}

func replaceMarkedBlock(existing, name string, entries []string, lifecycle models.Lifecycle, ownership models.OwnershipMode) (string, error) {
	start := "# >>> remotr authorized_keys " + name + " >>>"
	end := "# <<< remotr authorized_keys " + name + " <<<"
	lines := strings.Split(strings.TrimSuffix(existing, "\n"), "\n")
	if existing == "" {
		lines = nil
	}
	startAt, endAt := -1, -1
	for i, line := range lines {
		switch line {
		case start:
			if startAt >= 0 {
				return "", fmt.Errorf("duplicate managed authorized-key marker %q", name)
			}
			startAt = i
		case end:
			if startAt < 0 || endAt >= 0 {
				return "", fmt.Errorf("malformed managed authorized-key markers for %q", name)
			}
			endAt = i
		}
	}
	if (startAt < 0) != (endAt < 0) {
		return "", fmt.Errorf("malformed managed authorized-key markers for %q", name)
	}
	if lifecycle == models.LifecyclePresent && ownership == models.OwnershipMerge && startAt >= 0 {
		current := lines[startAt+1 : endAt]
		known := make(map[string]struct{}, len(current))
		for _, line := range current {
			known[line] = struct{}{}
		}
		for _, entry := range entries {
			if _, exists := known[entry]; !exists {
				current = append(current, entry)
			}
		}
		entries = current
	}
	if startAt >= 0 {
		lines = append(append([]string(nil), lines[:startAt]...), lines[endAt+1:]...)
	}
	if lifecycle == models.LifecyclePresent && len(entries) > 0 {
		block := append([]string{start}, entries...)
		block = append(block, end)
		lines = append(lines, block...)
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func contentMatches(current, desired string) bool {
	return current == desired || (desired == "" && strings.TrimSpace(current) == "")
}
