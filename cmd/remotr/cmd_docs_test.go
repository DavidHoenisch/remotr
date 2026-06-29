package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestOpenURLInBrowserNoOpener(t *testing.T) {
	origLookPath := lookPath
	lookPath = func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	t.Cleanup(func() { lookPath = origLookPath })

	opened, err := openURLInBrowser("https://example.com")
	if opened || err != nil {
		t.Fatalf("opened=%v err=%v", opened, err)
	}
}

func TestActionDocsPrintsURLWhenNoOpener(t *testing.T) {
	origLookPath := lookPath
	lookPath = func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	t.Cleanup(func() { lookPath = origLookPath })

	out := captureStdout(t, func() {
		if err := actionDocs(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	})
	if strings.TrimSpace(out) != remotrDocsURL {
		t.Fatalf("stdout = %q want %q", out, remotrDocsURL)
	}
}

func TestApp_docsCommand(t *testing.T) {
	origLookPath := lookPath
	lookPath = func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	t.Cleanup(func() { lookPath = origLookPath })

	app := newApp()
	out := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"remotr", "docs"}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.TrimSpace(out) != remotrDocsURL {
		t.Fatalf("stdout = %q want %q", out, remotrDocsURL)
	}
}
