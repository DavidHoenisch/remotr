package systemduser_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/systemduser"
)

func TestListInteractiveUsersFromPasswd(t *testing.T) {
	content := `root:x:0:0:root:/root:/bin/bash
nobody:x:65534:65534:Kernel Overflow User:/:/usr/bin/nologin
alice:x:1000:1000:Alice:/home/alice:/bin/bash
sync:x:5:0:sync:/sbin:/bin/sync
bob:x:1001:1001:Bob:/home/bob:/bin/bash
`
	users, err := systemduser.ListInteractiveUsersFromPasswd(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("users = %#v, want alice and bob", users)
	}
	if users[0].Username != "alice" || users[0].UID != 1000 {
		t.Fatalf("users[0] = %#v", users[0])
	}
	if users[1].Username != "bob" || users[1].UID != 1001 {
		t.Fatalf("users[1] = %#v", users[1])
	}
}

func TestListInteractiveUsersFromPasswd_skipsLowUIDAndNobody(t *testing.T) {
	content := `daemon:x:2:2:daemon:/sbin:/usr/sbin/nologin
nobody:x:999:999::/:/usr/sbin/nologin
svc:x:500:500:Service:/var/lib/svc:/usr/sbin/nologin
`
	users, err := systemduser.ListInteractiveUsersFromPasswd(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 0 {
		t.Fatalf("users = %#v, want none", users)
	}
}
