package models_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-LIA-014 & OS-LIA-015: structured sudo fragments support typed Defaults,
// multi-rule lists, user:group runAs targets, argument-scoped commandSpecs,
// optional lexical priority, and opt-in unmanaged-fragment auditing.
func TestParseState_canonicalSudoHardeningAndPriority(t *testing.T) {
	input := `schemaVersion: 1
configurations:
  - name: access
    resources:
      - kind: sudo
        name: platform-ops
        priority: 20
        lifecycle: present
        ownership: fragment
        recoveryPrincipals: [recovery]
        auditUnmanaged: true
        allowedUnmanagedFragments: [90-cloud-init-users, recovery]
        defaults:
          - option: use_pty
          - option: requiretty
            negate: true
          - scope: global
            option: timestamp_timeout
            value: "5"
          - scope: user
            target: "%ops"
            option: env_keep
            values: [SSH_AUTH_SOCK, LANG]
          - scope: command
            target: /usr/bin/systemctl
            option: log_output
        rules:
          - subjects: ["%ops"]
            runAs: ["root:root"]
            tags: [NOPASSWD]
            commandSpecs:
              - path: /usr/bin/id
                noArgs: true
              - path: /usr/bin/systemctl
                args: [status, nginx]
              - path: /usr/bin/su
                negate: true
          - subjects: [developer]
            runAs: ["ALL:ALL", ":adm"]
            tags: [PASSWD, NOEXEC]
            commands: [/usr/bin/journalctl]
`

	state, err := models.ParseState(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseState() error = %v", err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].Sudo) != 1 {
		t.Fatalf("parsed sudo count = %d, want 1", len(state.Configurations[0].Sudo))
	}
	got := state.Configurations[0].Sudo[0]
	if got.FragmentFilename() != "20-platform-ops" {
		t.Fatalf("FragmentFilename() = %q, want 20-platform-ops", got.FragmentFilename())
	}
	if len(got.Defaults) != 5 || len(got.Rules) != 2 || !got.AuditUnmanaged || len(got.AllowedUnmanagedFragments) != 2 {
		t.Fatalf("parsed sudo = %#v", got)
	}
}

func TestSudoResource_Validate(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	baseMeta := models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipFragment}

	cases := []struct {
		name     string
		resource models.SudoResource
		wantErr  bool
		wantFile string
	}{
		{
			name: "defaults-only hardening fragment with zero priority",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "hardening",
				Priority:           intPtr(0),
				RecoveryPrincipals: []string{"recovery"},
				Defaults: []models.SudoDefault{
					{Option: "use_pty"},
					{Option: "logfile", Value: "/var/log/sudo.log"},
					{Option: "umask", Value: "0027"},
					{Option: "secure_path", Value: "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
					{Option: "lecture", Value: "once"},
					{Option: "timestamp_type", Value: "tty"},
				},
			},
			wantFile: "00-hardening",
		},
		{
			name: "top-level single rule with commandSpecs and user:group runAs",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "diag",
				Subjects:           []string{"developer"},
				RunAs:              []string{"root:wheel", "ALL:ALL", ":adm"},
				CommandSpecs:       []models.SudoCommandSpec{{Path: "/usr/bin/id", NoArgs: true}},
				RecoveryPrincipals: []string{"recovery"},
			},
			wantFile: "diag",
		},
		{
			name: "absent lifecycle with priority only",
			resource: models.SudoResource{
				ResourceMeta:       models.ResourceMeta{Lifecycle: models.LifecycleAbsent, Ownership: models.OwnershipFragment},
				Name:               "legacy",
				Priority:           intPtr(99),
				RecoveryPrincipals: []string{"recovery"},
			},
			wantFile: "99-legacy",
		},
		{
			name: "priority negative rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "bad-prio",
				Priority:           intPtr(-1),
				RecoveryPrincipals: []string{"recovery"},
				Defaults:           []models.SudoDefault{{Option: "use_pty"}},
			},
			wantErr: true,
		},
		{
			name: "priority above 99 rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "bad-prio-high",
				Priority:           intPtr(100),
				RecoveryPrincipals: []string{"recovery"},
				Defaults:           []models.SudoDefault{{Option: "use_pty"}},
			},
			wantErr: true,
		},
		{
			name: "mixing top-level rule fields with rules list rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "mixed",
				Subjects:           []string{"developer"},
				Commands:           []string{"/usr/bin/id"},
				Rules:              []models.SudoRule{{Subjects: []string{"ops"}, Commands: []string{"/usr/bin/id"}}},
				RecoveryPrincipals: []string{"recovery"},
			},
			wantErr: true,
		},
		{
			name: "present without defaults or rules rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "empty",
				RecoveryPrincipals: []string{"recovery"},
			},
			wantErr: true,
		},
		{
			name: "unsupported Defaults option rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "bad-default",
				RecoveryPrincipals: []string{"recovery"},
				Defaults:           []models.SudoDefault{{Option: "editor", Value: "/bin/sh"}},
			},
			wantErr: true,
		},
		{
			name: "boolean Defaults with value rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "bool-with-val",
				RecoveryPrincipals: []string{"recovery"},
				Defaults:           []models.SudoDefault{{Option: "use_pty", Value: "true"}},
			},
			wantErr: true,
		},
		{
			name: "non-boolean Defaults with negate rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "int-negate",
				RecoveryPrincipals: []string{"recovery"},
				Defaults:           []models.SudoDefault{{Option: "timestamp_timeout", Value: "5", Negate: true}},
			},
			wantErr: true,
		},
		{
			name: "env_keep with unsafe env var rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "bad-env",
				RecoveryPrincipals: []string{"recovery"},
				Defaults:           []models.SudoDefault{{Option: "env_keep", Values: []string{"BAD VAR"}}},
			},
			wantErr: true,
		},
		{
			name: "global scope with target rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "global-target",
				RecoveryPrincipals: []string{"recovery"},
				Defaults:           []models.SudoDefault{{Scope: "global", Target: "alice", Option: "use_pty"}},
			},
			wantErr: true,
		},
		{
			name: "user scope without target rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "user-no-target",
				RecoveryPrincipals: []string{"recovery"},
				Defaults:           []models.SudoDefault{{Scope: "user", Option: "use_pty"}},
			},
			wantErr: true,
		},
		{
			name: "commandSpec with both args and noArgs rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "conflict-args",
				Subjects:           []string{"developer"},
				CommandSpecs:       []models.SudoCommandSpec{{Path: "/usr/bin/id", Args: []string{"-u"}, NoArgs: true}},
				RecoveryPrincipals: []string{"recovery"},
			},
			wantErr: true,
		},
		{
			name: "commandSpec with unsafe delimiter in arg rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "unsafe-arg",
				Subjects:           []string{"developer"},
				CommandSpecs:       []models.SudoCommandSpec{{Path: "/usr/bin/id", Args: []string{"-u,ALL"}}},
				RecoveryPrincipals: []string{"recovery"},
			},
			wantErr: true,
		},
		{
			name: "commandSpec with wildcard in path rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "wildcard-path",
				Subjects:           []string{"developer"},
				CommandSpecs:       []models.SudoCommandSpec{{Path: "/usr/bin/*"}},
				RecoveryPrincipals: []string{"recovery"},
			},
			wantErr: true,
		},
		{
			name: "invalid runAs user:group rejected",
			resource: models.SudoResource{
				ResourceMeta:       baseMeta,
				Name:               "bad-runas",
				Subjects:           []string{"developer"},
				RunAs:              []string{"root:bad:group"},
				Commands:           []string{"/usr/bin/id"},
				RecoveryPrincipals: []string{"recovery"},
			},
			wantErr: true,
		},
		{
			name: "allowedUnmanagedFragments without auditUnmanaged rejected",
			resource: models.SudoResource{
				ResourceMeta:              baseMeta,
				Name:                      "audit-off",
				Defaults:                  []models.SudoDefault{{Option: "use_pty"}},
				RecoveryPrincipals:        []string{"recovery"},
				AllowedUnmanagedFragments: []string{"90-cloud-init-users"},
			},
			wantErr: true,
		},
		{
			name: "absent lifecycle declaring defaults rejected",
			resource: models.SudoResource{
				ResourceMeta:       models.ResourceMeta{Lifecycle: models.LifecycleAbsent, Ownership: models.OwnershipFragment},
				Name:               "absent-defaults",
				Defaults:           []models.SudoDefault{{Option: "use_pty"}},
				RecoveryPrincipals: []string{"recovery"},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.resource.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && tc.wantFile != "" && tc.resource.FragmentFilename() != tc.wantFile {
				t.Fatalf("FragmentFilename() = %q, want %q", tc.resource.FragmentFilename(), tc.wantFile)
			}
		})
	}
}

func FuzzParseCanonicalSudoPolicy(f *testing.F) {
	f.Add("ops", 20, "use_pty", "", "root:root", "/usr/bin/id", "-u")
	f.Add("hardening", 0, "timestamp_timeout", "5", "ALL:ALL", "/usr/bin/systemctl", "status")
	f.Add("bad.name", 100, "editor", "/bin/sh", "bad:user:group", "relative/cmd", ",ALL")

	f.Fuzz(func(t *testing.T, name string, priority int, option, value, runAs, cmdPath, arg string) {
		if len(name) > 64 || len(option) > 64 || len(value) > 128 || len(runAs) > 64 || len(cmdPath) > 128 || len(arg) > 128 {
			return
		}
		doc := fmt.Sprintf(`schemaVersion: 1
configurations:
  - name: fuzz
    resources:
      - kind: sudo
        name: %q
        priority: %d
        lifecycle: present
        ownership: fragment
        recoveryPrincipals: [recovery]
        defaults:
          - option: %q
            value: %q
        rules:
          - subjects: [developer]
            runAs: [%q]
            commandSpecs:
              - path: %q
                args: [%q]
`, name, priority, option, value, runAs, cmdPath, arg)
		state, err := models.ParseState(strings.NewReader(doc))
		if err != nil {
			return
		}
		if len(state.Configurations) != 1 || len(state.Configurations[0].Sudo) != 1 {
			t.Fatalf("unexpected parsed sudo count: %#v", state)
		}
		got := state.Configurations[0].Sudo[0]
		if got.Priority != nil && (*got.Priority < 0 || *got.Priority > 99) {
			t.Fatalf("accepted out-of-range priority %d", *got.Priority)
		}
		if strings.ContainsAny(got.FragmentFilename(), "/\\.\r\n\x00") {
			t.Fatalf("accepted unsafe fragment filename %q", got.FragmentFilename())
		}
	})
}
