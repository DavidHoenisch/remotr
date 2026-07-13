package users_test

import (
	"context"
	"os/user"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/users"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_UIDDriftRequiresExplicitReassignment(t *testing.T) {
	a := users.New(models.UserResource{Name: "alice", Username: "alice", Present: true, UID: 2000})
	a.LookupFunc = func(string) (*user.User, error) { return &user.User{Username: "alice", Uid: "1000"}, nil }
	if _, met := a.State(context.Background()); met {
		t.Fatal("expected uid drift")
	}
	called := false
	a.ModifyUIDFunc = func(string, int) error { called = true; return nil }
	if err := a.Apply(context.Background()); err == nil {
		t.Fatal("expected reassignment policy error")
	}
	if called {
		t.Fatal("usermod called without explicit reassignment policy")
	}
}

func TestApplicator_UIDDriftReassignsExactUID(t *testing.T) {
	a := users.New(models.UserResource{Name: "alice", Username: "alice", Present: true, UID: 2000, AllowUIDReassignment: true})
	a.LookupFunc = func(string) (*user.User, error) { return &user.User{Username: "alice", Uid: "1000"}, nil }
	a.ModifyUIDFunc = func(name string, uid int) error {
		if name != "alice" || uid != 2000 {
			t.Fatalf("modify = %q %d", name, uid)
		}
		return nil
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
}
