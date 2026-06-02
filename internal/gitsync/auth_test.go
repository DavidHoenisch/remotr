package gitsync

import (
	"strings"
	"testing"
)

func TestAuthConfigArgs_githubPAT(t *testing.T) {
	gs := &GitSyncer{
		RemoteURL: "https://github.com/acme/remotr-config.git",
		Token:     "ghp_testtoken",
	}
	args := gs.authConfigArgs()
	if len(args) != 2 {
		t.Fatalf("args = %v", args)
	}
	if !strings.HasPrefix(args[0], "-c") {
		t.Fatalf("args = %v", args)
	}
	if !strings.Contains(args[1], "http.https://github.com/.extraHeader=Authorization: Basic") {
		t.Fatalf("args = %v", args[1])
	}
	if strings.Contains(args[1], "ghp_testtoken") {
		t.Fatal("token must not appear in config args")
	}
}

func TestAuthConfigArgs_noToken(t *testing.T) {
	gs := &GitSyncer{RemoteURL: "https://github.com/acme/remotr-config.git"}
	if args := gs.authConfigArgs(); args != nil {
		t.Fatalf("args = %v", args)
	}
}

func TestCleanRemoteURL_stripsCredentials(t *testing.T) {
	got := cleanRemoteURL("https://user:secret@github.com/acme/remotr-config.git")
	want := "https://github.com/acme/remotr-config.git"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestGitHost(t *testing.T) {
	if got := gitHost("https://github.com/acme/remotr-config.git"); got != "github.com" {
		t.Fatalf("host = %q", got)
	}
}
