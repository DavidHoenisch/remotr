package interactiveuser_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
)

func TestUsernamesFromAccounts_sortsByUID(t *testing.T) {
	got := interactiveuser.UsernamesFromAccounts([]interactiveuser.Account{
		{Username: "bob", UID: 1001},
		{Username: "alice", UID: 1000},
	})
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("got %#v", got)
	}
}

func TestJoinAndSplitUsernames(t *testing.T) {
	joined := interactiveuser.JoinUsernames([]string{"alice", "bob"})
	if joined != "alice,bob" {
		t.Fatalf("joined = %q", joined)
	}
	got := interactiveuser.SplitUsernames(joined)
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("split = %#v", got)
	}
	if interactiveuser.SplitUsernames("") != nil {
		t.Fatal("expected nil for empty")
	}
}

func TestListUsernamesFromPasswd(t *testing.T) {
	content := `alice:x:1000:1000:Alice:/home/alice:/bin/bash
bob:x:1001:1001:Bob:/home/bob:/bin/bash
`
	users, err := interactiveuser.ParsePasswd(content)
	if err != nil {
		t.Fatal(err)
	}
	got := interactiveuser.UsernamesFromAccounts(users)
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("got %#v", got)
	}
}
