package users

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/internal/userutil"
)

type Applicator struct {
	Resource          models.UserResource
	AddFunc           func(uname string) error
	DelFunc           func(uname string) error
	LookupFunc        func(string) (*user.User, error)
	AddUIDFunc        func(string, int) error
	ModifyUIDFunc     func(string, int) error
	LookupGroupFunc   func(string) (*user.Group, error)
	GroupIDsFunc      func(*user.User) ([]string, error)
	LookupShellFunc   func(string) (string, error)
	ShadowLookupFunc  func(string) (string, error)
	PasswordApplyFunc func(string, string) error
	ResolveSecret     func(context.Context, string) (string, error)
	LockLookupFunc    func(string) (bool, error)
	ExpiryLookupFunc  func(string) (string, error)
	RuntimeUsername   string
	ProtectedUserFunc func(string) bool
	Runner            executil.Runner
}

func New(r models.UserResource) *Applicator {
	applicator := &Applicator{
		Resource:        r,
		AddFunc:         userutil.Useradd,
		DelFunc:         userutil.Userdel,
		LookupFunc:      user.Lookup,
		AddUIDFunc:      userutil.UseraddUID,
		ModifyUIDFunc:   userutil.UsermodUID,
		LookupGroupFunc: user.LookupGroup,
		GroupIDsFunc:    func(user *user.User) ([]string, error) { return user.GroupIds() },
		Runner:          executil.SanitizedOSRunner{},
		ResolveSecret: func(_ context.Context, reference string) (string, error) {
			return secrets.ReadFileRef(reference)
		},
	}
	if current, err := user.Current(); err == nil {
		applicator.RuntimeUsername = current.Username
	}
	applicator.ProtectedUserFunc = defaultProtectedUser
	return applicator
}

func (a *Applicator) Name() string { return "user:" + a.Resource.Name }

func (a *Applicator) Description() string {
	if a.Resource.Present {
		return "ensure user " + a.Resource.Username
	}
	return "remove user " + a.Resource.Username
}

func (a *Applicator) lookup() (*user.User, error) {
	return a.LookupFunc(a.Resource.Username)
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	u, err := a.lookup()
	ex := err == nil
	if a.Resource.Present {
		if !ex {
			return nil, false
		}
		if a.Resource.UID > 0 {
			uid, parseErr := strconv.Atoi(u.Uid)
			if parseErr != nil || uid != a.Resource.UID {
				return u, false
			}
		}
		if !a.groupsMet(u) {
			return u, false
		}
		if !a.accountAttributesMet(u) {
			return u, false
		}
		if !a.passwordMet(ctx, u.Username) {
			return u, false
		}
		if !a.lockAndExpiryMet(u.Username) {
			return u, false
		}
		return u, true
	}
	return ex, !ex
}

func (a *Applicator) Apply(ctx context.Context) error {
	if !a.Resource.Present && (a.Resource.Username == a.RuntimeUsername || a.ProtectedUserFunc(a.Resource.Username)) {
		return fmt.Errorf("refusing to remove protected or Remotr runtime user %q", a.Resource.Username)
	}
	_, met := a.State(ctx)
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Resource.Present {
		u, err := a.lookup()
		if err != nil {
			if a.hasExtendedFields() {
				var passwordHash *string
				if a.Resource.PasswordHashRef != "" {
					resolved, err := a.resolvePasswordHash(ctx)
					if err != nil {
						return err
					}
					passwordHash = &resolved
				}
				if _, _, err := a.Runner.Run("useradd", a.createArgs()...); err != nil {
					return err
				}
				return a.applySensitiveWithPassword(ctx, true, passwordHash)
			}
			if a.Resource.UID > 0 {
				return a.AddUIDFunc(a.Resource.Username, a.Resource.UID)
			}
			return a.AddFunc(a.Resource.Username)
		}
		current, err := strconv.Atoi(u.Uid)
		if err != nil {
			return fmt.Errorf("user %q has invalid uid %q", a.Resource.Username, u.Uid)
		}
		if a.Resource.UID > 0 && current != a.Resource.UID {
			if !a.Resource.AllowUIDReassignment {
				return fmt.Errorf("user %q uid reassignment from %d to %d requires allowUIDReassignment", a.Resource.Username, current, a.Resource.UID)
			}
			if !a.hasExtendedFields() {
				return a.ModifyUIDFunc(a.Resource.Username, a.Resource.UID)
			}
		}
		args, err := a.modifyArgs(u)
		if err != nil {
			return err
		}
		if a.Resource.UID > 0 && current != a.Resource.UID {
			args = append([]string{"--uid", strconv.Itoa(a.Resource.UID)}, args...)
		}
		if len(args) > 0 {
			if _, _, err = a.Runner.Run("usermod", append(args, "--", a.Resource.Username)...); err != nil {
				return err
			}
		}
		return a.applySensitive(ctx, len(args) > 0)
	}
	if a.Resource.RemoveHome || a.Resource.ForceRemoval {
		args := make([]string, 0, 3)
		if a.Resource.RemoveHome {
			args = append(args, "--remove")
		}
		if a.Resource.ForceRemoval {
			args = append(args, "--force")
		}
		_, _, err := a.Runner.Run("userdel", append(args, "--", a.Resource.Username)...)
		return err
	}
	return a.DelFunc(a.Resource.Username)
}

func defaultProtectedUser(username string) bool {
	switch username {
	case "root", "daemon", "bin", "sys", "sync", "games", "man", "lp", "mail", "news", "uucp", "proxy", "www-data", "backup", "list", "irc", "gnats", "nobody":
		return true
	default:
		return false
	}
}

func (a *Applicator) hasExtendedFields() bool {
	return a.hasManagedGroups() || a.Resource.Home != "" || a.Resource.CreateHome != nil || a.Resource.Shell != "" || a.Resource.Comment != "" || a.Resource.System != nil || a.Resource.PasswordHashRef != ""
}

func (a *Applicator) createArgs() []string {
	args := make([]string, 0, 14)
	if a.Resource.System != nil && *a.Resource.System {
		args = append(args, "--system")
	}
	if a.Resource.UID > 0 {
		args = append(args, "--uid", strconv.Itoa(a.Resource.UID))
	}
	if a.Resource.PrimaryGroup != "" {
		args = append(args, "--gid", a.Resource.PrimaryGroup)
	}
	if a.Resource.SupplementaryGroupsMode != "" {
		args = append(args, "--groups", strings.Join(a.Resource.SupplementaryGroups, ","))
	}
	if a.Resource.Home != "" {
		args = append(args, "--home", a.Resource.Home)
	}
	if a.Resource.CreateHome != nil {
		if *a.Resource.CreateHome {
			args = append(args, "--create-home")
		} else {
			args = append(args, "--no-create-home")
		}
	}
	if a.Resource.Shell != "" {
		args = append(args, "--shell", a.Resource.Shell)
	}
	if a.Resource.Comment != "" {
		args = append(args, "--comment", a.Resource.Comment)
	}
	return append(args, "--", a.Resource.Username)
}

func (a *Applicator) hasManagedGroups() bool {
	return a.Resource.PrimaryGroup != "" || a.Resource.SupplementaryGroupsMode != ""
}

func (a *Applicator) groupsMet(account *user.User) bool {
	if !a.hasManagedGroups() {
		return true
	}
	primaryID, err := strconv.Atoi(account.Gid)
	if err != nil {
		return false
	}
	if a.Resource.PrimaryGroup != "" {
		group, err := a.LookupGroupFunc(a.Resource.PrimaryGroup)
		if err != nil || group == nil {
			return false
		}
		wanted, err := strconv.Atoi(group.Gid)
		if err != nil || wanted != primaryID {
			return false
		}
	}
	if a.Resource.SupplementaryGroupsMode == "" {
		return true
	}
	groupIDs, err := a.GroupIDsFunc(account)
	if err != nil {
		return false
	}
	actual := make(map[string]struct{}, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID != account.Gid {
			actual[groupID] = struct{}{}
		}
	}
	wanted := make(map[string]struct{}, len(a.Resource.SupplementaryGroups))
	for _, name := range a.Resource.SupplementaryGroups {
		group, err := a.LookupGroupFunc(name)
		if err != nil || group == nil {
			return false
		}
		wanted[group.Gid] = struct{}{}
		if _, exists := actual[group.Gid]; !exists {
			return false
		}
	}
	if a.Resource.SupplementaryGroupsMode == models.GroupMembershipAuthoritative && len(actual) != len(wanted) {
		return false
	}
	return true
}

func (a *Applicator) modifyArgs(account *user.User) ([]string, error) {
	args := make([]string, 0, 11)
	if a.Resource.PrimaryGroup != "" {
		group, err := a.LookupGroupFunc(a.Resource.PrimaryGroup)
		if err != nil || group == nil {
			return nil, fmt.Errorf("primary group %q: %w", a.Resource.PrimaryGroup, err)
		}
		if account.Gid != group.Gid {
			args = append(args, "--gid", a.Resource.PrimaryGroup)
		}
	}
	if a.Resource.SupplementaryGroupsMode != "" && !a.groupsMet(account) {
		if a.Resource.SupplementaryGroupsMode == models.GroupMembershipMerge {
			args = append(args, "--append")
		}
		args = append(args, "--groups", strings.Join(a.Resource.SupplementaryGroups, ","))
	}
	if a.Resource.Home != "" && account.HomeDir != a.Resource.Home {
		args = append(args, "--home", a.Resource.Home)
	}
	if a.Resource.Shell != "" {
		shell, err := a.lookupShell(account.Username)
		if err != nil {
			return nil, err
		}
		if shell != a.Resource.Shell {
			args = append(args, "--shell", a.Resource.Shell)
		}
	}
	if a.Resource.Comment != "" && account.Name != a.Resource.Comment {
		args = append(args, "--comment", a.Resource.Comment)
	}
	if a.Resource.System != nil {
		current, err := strconv.Atoi(account.Uid)
		if err != nil {
			return nil, err
		}
		if (current < 1000) != *a.Resource.System && (a.Resource.UID == 0 || (a.Resource.UID < 1000) != *a.Resource.System) {
			return nil, fmt.Errorf("user %q system class cannot be changed without a matching uid reassignment", a.Resource.Username)
		}
	}
	return args, nil
}

func (a *Applicator) accountAttributesMet(account *user.User) bool {
	if a.Resource.Home != "" && account.HomeDir != a.Resource.Home {
		return false
	}
	if a.Resource.Comment != "" && account.Name != a.Resource.Comment {
		return false
	}
	if a.Resource.System != nil {
		uid, err := strconv.Atoi(account.Uid)
		if err != nil || (uid < 1000) != *a.Resource.System {
			return false
		}
	}
	if a.Resource.Shell != "" {
		shell, err := a.lookupShell(account.Username)
		if err != nil || shell != a.Resource.Shell {
			return false
		}
	}
	return true
}

func (a *Applicator) lookupShell(username string) (string, error) {
	if a.LookupShellFunc != nil {
		return a.LookupShellFunc(username)
	}
	stdout, _, err := a.Runner.Run("getent", "passwd", username)
	if err != nil {
		return "", err
	}
	fields := strings.Split(strings.TrimSpace(string(stdout)), ":")
	if len(fields) != 7 || fields[0] != username {
		return "", fmt.Errorf("invalid passwd lookup result for %q", username)
	}
	return fields[6], nil
}

func (a *Applicator) passwordMet(ctx context.Context, username string) bool {
	if a.Resource.PasswordHashRef == "" {
		return true
	}
	desired, err := a.resolvePasswordHash(ctx)
	if err != nil {
		return false
	}
	observed, err := a.lookupShadowHash(username)
	return err == nil && passwordHashesEqual(observed, desired)
}

func (a *Applicator) applySensitive(ctx context.Context, changed bool) error {
	return a.applySensitiveWithPassword(ctx, changed, nil)
}

func (a *Applicator) applySensitiveWithPassword(ctx context.Context, changed bool, resolvedPasswordHash *string) error {
	if a.Resource.PasswordHashRef != "" {
		hash := ""
		var err error
		if resolvedPasswordHash != nil {
			hash = *resolvedPasswordHash
		} else {
			hash, err = a.resolvePasswordHash(ctx)
		}
		if err != nil {
			return err
		}
		observed, lookupErr := a.lookupShadowHash(a.Resource.Username)
		if lookupErr != nil || !passwordHashesEqual(observed, hash) {
			if a.PasswordApplyFunc != nil {
				err = a.PasswordApplyFunc(a.Resource.Username, hash)
			} else if runner, ok := a.Runner.(executil.InputRunner); ok {
				_, _, err = runner.RunInput("chpasswd", []byte(a.Resource.Username+":"+hash+"\n"), "--encrypted")
			} else {
				err = fmt.Errorf("password update requires protected stdin runner")
			}
			if err != nil {
				return err
			}
			changed = true
		}
	}
	if a.Resource.Locked != nil {
		locked, err := a.lookupLocked(a.Resource.Username)
		if err != nil {
			return err
		}
		if locked != *a.Resource.Locked {
			flag := "--unlock"
			if *a.Resource.Locked {
				flag = "--lock"
			}
			if _, _, err := a.Runner.Run("usermod", flag, "--", a.Resource.Username); err != nil {
				return err
			}
			changed = true
		}
	}
	if a.Resource.Expiry != "" {
		expiry, err := a.lookupExpiry(a.Resource.Username)
		if err != nil {
			return err
		}
		if expiry != a.Resource.Expiry {
			value := a.Resource.Expiry
			if value == "never" {
				value = "-1"
			}
			if _, _, err := a.Runner.Run("chage", "--expiredate", value, "--", a.Resource.Username); err != nil {
				return err
			}
			changed = true
		}
	}
	if !changed {
		return appErr.ErrStateAlreadyMet
	}
	return nil
}

func (a *Applicator) resolvePasswordHash(ctx context.Context) (string, error) {
	hash, err := a.ResolveSecret(ctx, a.Resource.PasswordHashRef)
	if err != nil {
		return "", fmt.Errorf("resolve password hash for user %q: %w", a.Resource.Username, secrets.RedactedResolutionError(err))
	}
	return hash, nil
}

func passwordHashesEqual(observed, desired string) bool {
	if observed == desired {
		return true
	}
	wantedHash := strings.TrimLeft(desired, "!")
	return wantedHash != "" && strings.TrimLeft(observed, "!") == wantedHash
}

func (a *Applicator) lockAndExpiryMet(username string) bool {
	if a.Resource.Locked != nil {
		locked, err := a.lookupLocked(username)
		if err != nil || locked != *a.Resource.Locked {
			return false
		}
	}
	if a.Resource.Expiry != "" {
		expiry, err := a.lookupExpiry(username)
		if err != nil || expiry != a.Resource.Expiry {
			return false
		}
	}
	return true
}

func (a *Applicator) lookupLocked(username string) (bool, error) {
	if a.LockLookupFunc != nil {
		return a.LockLookupFunc(username)
	}
	stdout, _, err := a.Runner.Run("passwd", "--status", username)
	if err != nil {
		return false, err
	}
	fields := strings.Fields(string(stdout))
	if len(fields) < 2 || fields[0] != username {
		return false, fmt.Errorf("invalid password status for %q", username)
	}
	return fields[1] == "L", nil
}

func (a *Applicator) lookupExpiry(username string) (string, error) {
	if a.ExpiryLookupFunc != nil {
		return a.ExpiryLookupFunc(username)
	}
	stdout, _, err := a.Runner.Run("chage", "--list", "--iso8601", username)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(stdout), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "Account expires" {
			value = strings.TrimSpace(value)
			if value == "never" || value != "" {
				return value, nil
			}
		}
	}
	return "", fmt.Errorf("account expiry missing for %q", username)
}

func (a *Applicator) lookupShadowHash(username string) (string, error) {
	if a.ShadowLookupFunc != nil {
		return a.ShadowLookupFunc(username)
	}
	stdout, _, err := a.Runner.Run("getent", "shadow", username)
	if err != nil {
		return "", err
	}
	fields := strings.Split(strings.TrimSpace(string(stdout)), ":")
	if len(fields) < 2 || fields[0] != username {
		return "", fmt.Errorf("invalid shadow lookup result for %q", username)
	}
	return fields[1], nil
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

// Exists reports whether the username is present.
func Exists(username string) bool {
	_, err := user.Lookup(username)
	if errors.As(err, new(user.UnknownUserError)) {
		return false
	}
	return err == nil
}
