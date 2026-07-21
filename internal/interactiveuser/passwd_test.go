package interactiveuser_test

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParsePasswd_interactiveUsers(t *testing.T) {
	content := `root:x:0:0:root:/root:/bin/bash
nobody:x:65534:65534:Kernel Overflow User:/:/usr/bin/nologin
century:x:100:100:Century:/home/century:/bin/bash
alice:x:1000:1000:Alice:/home/alice:/bin/bash
sync:x:5:0:sync:/sbin:/bin/sync
bob:x:1001:1001:Bob:/home/bob:/bin/bash
`
	users, err := interactiveuser.ParsePasswd(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("users = %#v, want alice and bob", users)
	}
	if users[0].Username != "alice" || users[0].UID != 1000 || users[0].HomeDir != "/home/alice" {
		t.Fatalf("users[0] = %#v", users[0])
	}
}

func TestParsePasswd_skipsLowUIDNobodyAndSystemAccounts(t *testing.T) {
	content := `daemon:x:2:2:daemon:/sbin:/usr/sbin/nologin
nobody:x:999:999::/:/usr/sbin/nologin
svc:x:500:500:Service:/var/lib/svc:/usr/sbin/nologin
systemd-timesync:x:997:997:systemd Time Synchronization:/:/usr/sbin/nologin
alice:x:1000:1000:Alice:/home/alice:/bin/bash
`
	users, err := interactiveuser.ParsePasswd(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("users = %#v, want alice only", users)
	}
}

// OS-AEC-098: desktop/session selection is account based, not session based.
// All-interactive therefore includes both live and logged-out eligible accounts;
// explicit selection retains authored order and reports missing names without
// broadening to another account.
func TestSelectInteractiveUsersDoesNotDependOnLoginStateOrBroaden(t *testing.T) {
	accounts := []interactiveuser.Account{
		{Username: "logged-in", UID: 1000, GID: 1000, HomeDir: "/home/logged-in"},
		{Username: "logged-out", UID: 1001, GID: 1001, HomeDir: "/home/logged-out"},
	}

	selected, unresolved, err := interactiveuser.Select(accounts, models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll})
	if err != nil {
		t.Fatal(err)
	}
	if got := interactiveuser.UsernamesFromAccounts(selected); !slices.Equal(got, []string{"logged-in", "logged-out"}) || len(unresolved) != 0 {
		t.Fatalf("all-interactive selection = %v unresolved %v, want both accounts", got, unresolved)
	}

	selected, unresolved, err = interactiveuser.Select(accounts, models.InteractiveUserSelector{
		Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"logged-out", "missing", "logged-in"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(selected))
	for _, account := range selected {
		got = append(got, account.Username)
	}
	if !slices.Equal(got, []string{"logged-out", "logged-in"}) || !slices.Equal(unresolved, []string{"missing"}) {
		t.Fatalf("explicit selection = %v unresolved %v, want authored matches and missing target", got, unresolved)
	}
}

func TestHomePath(t *testing.T) {
	got, err := interactiveuser.HomePath("/home/alice", ".config/app/settings.yaml")
	if err != nil || got != "/home/alice/.config/app/settings.yaml" {
		t.Fatalf("got %q err %v", got, err)
	}
	_, err = interactiveuser.HomePath("/home/alice", "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	_, err = interactiveuser.HomePath("/home/alice", "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for traversal")
	}
}
