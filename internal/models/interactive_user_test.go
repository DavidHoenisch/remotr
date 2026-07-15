package models_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestInteractiveUserSelectorValidation(t *testing.T) {
	tests := []struct {
		name     string
		selector models.InteractiveUserSelector
		wantErr  bool
	}{
		{name: "all", selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll}},
		{name: "explicit", selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice", "bob"}}},
		{name: "all with names", selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll, Usernames: []string{"alice"}}, wantErr: true},
		{name: "empty explicit", selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit}, wantErr: true},
		{name: "duplicate explicit", selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice", "alice"}}, wantErr: true},
		{name: "invalid username", selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"../alice"}}, wantErr: true},
		{name: "unknown mode", selector: models.InteractiveUserSelector{Mode: "labels"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.selector.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func FuzzInteractiveUserSelectorValidation(f *testing.F) {
	f.Add("all-interactive", "")
	f.Add("explicit", "alice")
	f.Add("explicit", "../root")
	f.Fuzz(func(t *testing.T, mode, username string) {
		if len(mode) > 64 || len(username) > 128 {
			// test-exception: EXC-014
			t.Skip()
		}
		selector := models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionMode(mode)}
		if username != "" {
			selector.Usernames = []string{username}
		}
		err := selector.Validate()
		if err == nil {
			if selector.Mode == models.InteractiveUserSelectionAll && len(selector.Usernames) != 0 {
				t.Fatal("all-interactive selector accepted usernames")
			}
			if selector.Mode == models.InteractiveUserSelectionExplicit && len(selector.Usernames) == 0 {
				t.Fatal("explicit selector accepted no usernames")
			}
		}
	})
}

func TestUserFileSelectorOwnershipValidation(t *testing.T) {
	base := models.UserFileResource{
		Name: "motd", Selector: &models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll},
		Path: ".motd", Content: "managed\n",
	}
	for _, ownership := range []models.OwnershipMode{"", models.OwnershipMerge, models.OwnershipAuthoritative} {
		resource := base
		resource.Ownership = ownership
		if err := resource.Validate(); err != nil {
			t.Fatalf("ownership %q: %v", ownership, err)
		}
	}
	base.Ownership = models.OwnershipNamed
	if err := base.Validate(); err == nil {
		t.Fatal("named ownership was accepted for selector cleanup")
	}
}
