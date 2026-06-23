package registry

import "testing"

func TestMemory_SetAndDeleteEndpointLabel(t *testing.T) {
	m := NewMemory()
	if err := m.RegisterEndpoint(Endpoint{ID: "laptop-01", Fleet: "dev"}); err != nil {
		t.Fatal(err)
	}

	labels, err := m.SetEndpointLabel("laptop-01", "site", "berlin")
	if err != nil {
		t.Fatal(err)
	}
	if labels["site"] != "berlin" {
		t.Fatalf("labels = %+v", labels)
	}

	removed, err := m.DeleteEndpointLabel("laptop-01", "site")
	if err != nil || !removed {
		t.Fatalf("DeleteEndpointLabel = %v %v", removed, err)
	}
	ep, ok, _ := m.GetEndpoint("laptop-01")
	if !ok || len(ep.Labels) != 0 {
		t.Fatalf("labels after delete = %+v", ep.Labels)
	}
}

func TestMemory_SetEndpointLabel_unknownEndpoint(t *testing.T) {
	m := NewMemory()
	if _, err := m.SetEndpointLabel("missing", "site", "berlin"); err != ErrEndpointNotFound {
		t.Fatalf("err = %v", err)
	}
}
