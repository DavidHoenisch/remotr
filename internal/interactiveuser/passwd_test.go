package interactiveuser_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
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
	if len(users) != 3 {
		t.Fatalf("users = %#v, want century alice bob", users)
	}
	if users[0].Username != "century" || users[0].UID != 100 || users[0].HomeDir != "/home/century" {
		t.Fatalf("users[0] = %#v", users[0])
	}
}

func TestParsePasswd_skipsLowUIDAndNobody(t *testing.T) {
	content := `daemon:x:2:2:daemon:/sbin:/usr/sbin/nologin
nobody:x:999:999::/:/usr/sbin/nologin
svc:x:500:500:Service:/var/lib/svc:/usr/sbin/nologin
`
	users, err := interactiveuser.ParsePasswd(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != "svc" {
		t.Fatalf("users = %#v, want svc only", users)
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
