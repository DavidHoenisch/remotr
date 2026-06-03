package main

import (
	"testing"
)

func TestPeelJSONFlag(t *testing.T) {
	got, rest := peelJSONFlag([]string{"phalanx-acae925c", "--json"})
	if !got || len(rest) != 1 || rest[0] != "phalanx-acae925c" {
		t.Fatalf("got=%v rest=%v", got, rest)
	}
}

func TestEndpointPositionalID_afterFlags(t *testing.T) {
	args := []string{"phalanx-acae925c", "--server-url", "https://example"}
	id, err := endpointPositionalID(nil, args)
	if err != nil || id != "phalanx-acae925c" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestEndpointPositionalID_fromParsedArgs(t *testing.T) {
	id, err := endpointPositionalID([]string{"phalanx-acae925c"}, nil)
	if err != nil || id != "phalanx-acae925c" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}
