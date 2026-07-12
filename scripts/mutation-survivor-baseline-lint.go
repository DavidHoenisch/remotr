//go:build ignore

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type baseline struct {
	SchemaVersion int `json:"schemaVersion"`
	Campaign      struct {
		Tool          string `json:"tool"`
		ToolVersion   string `json:"toolVersion"`
		SourceCommit  string `json:"sourceCommit"`
		ConfigSHA256  string `json:"configSHA256"`
		RecordedAt    string `json:"recordedAt"`
		SurvivorCount int    `json:"survivorCount"`
	} `json:"campaign"`
	Survivors []survivor `json:"survivors"`
}

type survivor struct {
	Key          string `json:"key"`
	MewtID       int    `json:"mewtID"`
	Target       string `json:"target"`
	TargetSHA256 string `json:"targetSHA256"`
	Mutation     struct {
		Slug          string `json:"slug"`
		ByteOffset    int    `json:"byteOffset"`
		OldTextSHA256 string `json:"oldTextSHA256"`
		NewTextSHA256 string `json:"newTextSHA256"`
	} `json:"mutation"`
	Outcome     string `json:"outcome"`
	Disposition struct {
		Status     string `json:"status"`
		Owner      string `json:"owner"`
		Reviewer   string `json:"reviewer"`
		ReviewedAt string `json:"reviewedAt"`
		Reason     string `json:"reason"`
		ExpiresAt  string `json:"expiresAt"`
	} `json:"disposition"`
	Reproduction struct {
		Command        string `json:"command"`
		ExpectedStatus string `json:"expectedStatus"`
		VerifiedAt     string `json:"verifiedAt"`
	} `json:"reproduction"`
}

func main() {
	path := flag.String("baseline", "test/mutation/survivor-baseline.json", "survivor baseline JSON")
	flag.Parse()

	data, err := os.ReadFile(*path)
	if err != nil {
		fail("read %s: %v", *path, err)
	}
	var got baseline
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&got); err != nil {
		fail("parse %s: %v", *path, err)
	}
	if got.SchemaVersion != 1 {
		fail("schemaVersion = %d, want 1", got.SchemaVersion)
	}
	if got.Campaign.Tool != "mewt" || got.Campaign.ToolVersion == "" || got.Campaign.SourceCommit == "" || !isSHA256(got.Campaign.ConfigSHA256) || got.Campaign.RecordedAt == "" {
		fail("campaign must name Mewt, version, source commit, config SHA-256, and recording date")
	}
	if got.Campaign.SurvivorCount < len(got.Survivors) {
		fail("survivorCount %d is less than %d recorded survivor entries", got.Campaign.SurvivorCount, len(got.Survivors))
	}

	seen := make(map[string]bool, len(got.Survivors))
	for _, s := range got.Survivors {
		if seen[s.Key] || s.Key == "" {
			fail("survivor key %q is missing or duplicated", s.Key)
		}
		seen[s.Key] = true
		if s.MewtID <= 0 || s.Target == "" || !isSHA256(s.TargetSHA256) || s.Mutation.Slug == "" || s.Mutation.ByteOffset < 0 || !isSHA256(s.Mutation.OldTextSHA256) || !isSHA256(s.Mutation.NewTextSHA256) {
			fail("survivor %q has incomplete stable mutant identity", s.Key)
		}
		if s.Outcome != "Uncaught" || s.Reproduction.ExpectedStatus != "Uncaught" || s.Reproduction.VerifiedAt == "" || !strings.Contains(s.Reproduction.Command, fmt.Sprintf("test --ids %d", s.MewtID)) {
			fail("survivor %q must record an Uncaught individual-mutant reproduction", s.Key)
		}
		if s.Disposition.Owner == "" {
			fail("survivor %q has no owner", s.Key)
		}
		switch s.Disposition.Status {
		case "untriaged", "test-gap":
		case "equivalent", "intentional", "tooling-failure":
			if s.Disposition.Reviewer == "" || s.Disposition.ReviewedAt == "" || s.Disposition.Reason == "" || s.Disposition.ExpiresAt == "" {
				fail("accepted survivor %q needs reviewer, review date, reason, and expiry", s.Key)
			}
		default:
			fail("survivor %q has invalid disposition %q", s.Key, s.Disposition.Status)
		}
		content, err := os.ReadFile(s.Target)
		if err != nil {
			fail("read target for %q: %v", s.Key, err)
		}
		if hash(content) != s.TargetSHA256 {
			fail("target SHA-256 for %q is stale", s.Key)
		}
	}
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mutation survivor baseline: "+format+"\n", args...)
	os.Exit(1)
}
