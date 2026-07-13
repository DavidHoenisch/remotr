package acceptance

import (
	"context"
	"errors"
	"os"
	osuser "os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/authorizedkeys"
	"github.com/DavidHoenisch/remotr/internal/applicators/groups"
	"github.com/DavidHoenisch/remotr/internal/applicators/sudo"
	"github.com/DavidHoenisch/remotr/internal/applicators/users"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const m2AdministratorKey = "AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W"
func TestLocalAdministratorLifecycleFeature(t *testing.T) {
	state := &localAdministratorState{}
	RunFeatureFiles(t, []string{"features/local_administrator.feature"}, func(steps *ScenarioSteps) {
		steps.Step(`^a declarative M2 local-administrator configuration$`, state.configure)
		steps.Step(`^the agent provisions the local administrator$`, state.provision)
		steps.Step(`^the local administrator has only Remotr-owned access$`, state.assertProvisioned)
		steps.Step(`^the agent revokes the local administrator$`, state.revoke)
		steps.Step(`^the account and Remotr-owned access are absent$`, state.assertRevoked)
	})
}

type localAdministratorState struct {
	config      models.Configuration
	home        string
	sudoersDir  string
	sudoersPath string
	userExists  bool
	groupRunner *executil.MockRunner
}

func (s *localAdministratorState) configure() error {
	artifact := `schemaVersion: 1
configurations:
  - name: m2-access
    resources:
      - kind: group
        name: administrators
        lifecycle: present
        group: administrators
      - kind: user
        name: developer
        username: developer
        present: true
      - kind: authorizedKey
        name: developer-access
        lifecycle: present
        ownership: authoritative
        enforce: true
        user: developer
        recoveryPrincipals: [recovery]
        entries:
          - type: ssh-ed25519
            key: AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W
            fingerprint: SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4
            restrictions: [no-agent-forwarding]
      - kind: sudo
        name: developer-admin
        lifecycle: present
        ownership: fragment
        enforce: true
        subjects: [developer]
        runAs: [ALL]
        commands: [/usr/bin/id]
        tags: [NOPASSWD]
        recoveryPrincipals: [recovery]
`
	state, err := models.ParseState(strings.NewReader(artifact))
	if err != nil {
		return err
	}
	if err := configrepo.ValidateState(state, "m2-local-administrator"); err != nil {
		return err
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].Commands) != 0 {
		return errors.New("local administrator flow must not use generic command resources")
	}
	s.config = state.Configurations[0]
	s.home, err = os.MkdirTemp("", "remotr-m2-admin-home-")
	if err != nil {
		return err
	}
	s.sudoersDir = filepath.Join(s.home, "sudoers.d")
	if err := os.Mkdir(s.sudoersDir, 0o750); err != nil {
		return err
	}
	s.sudoersPath = filepath.Join(s.home, "sudoers")
	return os.WriteFile(s.sudoersPath, []byte("#includedir "+s.sudoersDir+"\n"), 0o440)
}

func (s *localAdministratorState) provision() error {
	s.groupRunner = &executil.MockRunner{Next: map[string]executil.MockResult{
		"getent [group administrators]": {Err: errors.New("group is absent")},
		"groupadd [-- administrators]":  {},
	}}
	if err := groups.New(s.config.Groups[0], s.groupRunner).Apply(context.Background()); err != nil {
		return err
	}
	userProvider := users.New(s.config.Users[0])
	userProvider.LookupFunc = s.lookupUser
	userProvider.AddFunc = func(string) error { s.userExists = true; return nil }
	if err := userProvider.Apply(context.Background()); err != nil {
		return err
	}
	authorized := authorizedkeys.New(s.config.AuthorizedKeys[0])
	authorized.LookupUser = s.lookupInteractiveUser
	authorized.RecoveryCheck = func(string) error { return nil }
	if err := authorized.Apply(context.Background()); err != nil {
		return err
	}
	sudoProvider := sudo.New(s.config.Sudo[0])
	sudoProvider.SudoersDir, sudoProvider.SudoersPath = s.sudoersDir, s.sudoersPath
	sudoProvider.LookupRecovery = func(string) error { return nil }
	sudoProvider.ValidateEffective = func(context.Context, string, string) error { return nil }
	return sudoProvider.Apply(context.Background())
}

func (s *localAdministratorState) assertProvisioned() error {
	if !s.userExists || s.groupRunner == nil {
		return errors.New("administrator account or group was not provisioned")
	}
	var sawGroupAdd bool
	for _, call := range s.groupRunner.Calls {
		if call.Name == "groupadd" {
			sawGroupAdd = true
		}
	}
	if !sawGroupAdd {
		return errors.New("group provider did not provision the administrator group")
	}
	keyFile := filepath.Join(s.home, ".ssh", "authorized_keys")
	keys, err := os.ReadFile(keyFile)
	if err != nil || !strings.Contains(string(keys), "# >>> remotr authorized_keys developer-access >>>") || !strings.Contains(string(keys), m2AdministratorKey) {
		return errors.New("authorized-key provider did not create its owned administrator access block")
	}
	fragment, err := os.ReadFile(filepath.Join(s.sudoersDir, "developer-admin"))
	if err != nil || string(fragment) != "developer ALL=(ALL) NOPASSWD: /usr/bin/id\n" {
		return errors.New("sudo provider did not create the owned administrator fragment")
	}
	return nil
}

func (s *localAdministratorState) revoke() error {
	authorizedResource := s.config.AuthorizedKeys[0]
	authorizedResource.Lifecycle, authorizedResource.Entries = models.LifecycleAbsent, nil
	authorized := authorizedkeys.New(authorizedResource)
	authorized.LookupUser = s.lookupInteractiveUser
	authorized.RecoveryCheck = func(string) error { return nil }
	if err := authorized.Apply(context.Background()); err != nil {
		return err
	}
	sudoResource := s.config.Sudo[0]
	sudoResource.Lifecycle = models.LifecycleAbsent
	sudoProvider := sudo.New(sudoResource)
	sudoProvider.SudoersDir, sudoProvider.SudoersPath = s.sudoersDir, s.sudoersPath
	sudoProvider.LookupRecovery = func(string) error { return nil }
	sudoProvider.ValidateEffective = func(context.Context, string, string) error { return nil }
	if err := sudoProvider.Apply(context.Background()); err != nil {
		return err
	}
	userResource := s.config.Users[0]
	userResource.Present = false
	userProvider := users.New(userResource)
	userProvider.LookupFunc = s.lookupUser
	userProvider.DelFunc = func(string) error { s.userExists = false; return nil }
	return userProvider.Apply(context.Background())
}

func (s *localAdministratorState) assertRevoked() error {
	if s.userExists {
		return errors.New("administrator account remains")
	}
	keys, err := os.ReadFile(filepath.Join(s.home, ".ssh", "authorized_keys"))
	if err != nil || strings.TrimSpace(string(keys)) != "" {
		return errors.New("Remotr-owned authorized-key access remains")
	}
	if _, err := os.Stat(filepath.Join(s.sudoersDir, "developer-admin")); !os.IsNotExist(err) {
		return errors.New("Remotr-owned sudo fragment remains")
	}
	return nil
}

func (s *localAdministratorState) lookupUser(string) (*osuser.User, error) {
	if !s.userExists {
		return nil, os.ErrNotExist
	}
	return &osuser.User{Username: "developer", Uid: strconv.Itoa(os.Getuid()), Gid: strconv.Itoa(os.Getgid()), HomeDir: s.home}, nil
}

func (s *localAdministratorState) lookupInteractiveUser(string) (interactiveuser.Account, error) {
	if !s.userExists {
		return interactiveuser.Account{}, os.ErrNotExist
	}
	return interactiveuser.Account{Username: "developer", UID: os.Getuid(), GID: os.Getgid(), HomeDir: s.home}, nil
}
