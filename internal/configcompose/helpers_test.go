package configcompose_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
)

func kindModule(body string) string {
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "kind:") {
		return body + "\n"
	}
	return "kind: module\n" + body + "\n"
}

func kindManifest(body string) string {
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "kind:") {
		return body + "\n"
	}
	return "kind: manifest\n" + body + "\n"
}

func kindApplication(body string) string {
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "kind:") {
		return body + "\n"
	}
	return "kind: application\n" + body + "\n"
}

func kindCrons(body string) string {
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "kind:") {
		return body + "\n"
	}
	return "kind: crons\n" + body + "\n"
}

func renderFleetBody(t *testing.T, dir, fleet string) string {
	t.Helper()
	desired, _, _, _, err := configcompose.RenderFleet(dir, fleet)
	if err != nil {
		t.Fatal(err)
	}
	return string(desired)
}

func renderEndpointBody(t *testing.T, dir, endpointID string) string {
	t.Helper()
	desired, _, _, _, err := configcompose.RenderEndpoint(dir, endpointID)
	if err != nil {
		t.Fatal(err)
	}
	return string(desired)
}

func validateComposition(t *testing.T, dir string) configcompose.Result {
	t.Helper()
	res, err := configcompose.ValidateComposition(dir)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
