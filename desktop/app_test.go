package main

import (
	"encoding/json"
	"testing"
)

func TestApplicationInfoBindingExposesOnlySafeIdentity(t *testing.T) {
	app := NewApp("v1.2.3")

	encoded, err := json.Marshal(app.GetApplicationInfo())
	if err != nil {
		t.Fatalf("marshal application info: %v", err)
	}
	const want = `{"name":"Remotr Desktop","version":"v1.2.3"}`
	if string(encoded) != want {
		t.Fatalf("GetApplicationInfo() = %s, want %s", encoded, want)
	}

	options := newApplicationOptions(app)
	if len(options.Bind) != 1 || options.Bind[0] != app {
		t.Fatalf("bound services = %#v, want only the application service", options.Bind)
	}
}
