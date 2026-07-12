package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryHandlerRequiresTokenAndRecordsAcknowledgement(t *testing.T) {
	result := filepath.Join(t.TempDir(), "acknowledgement")
	handler := recoveryHandler("synthetic-token", result)

	health := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthResponse := httptest.NewRecorder()
	handler.ServeHTTP(healthResponse, health)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", healthResponse.Code, http.StatusOK)
	}

	unauthorized := httptest.NewRequest(http.MethodPost, "/ack", nil)
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized acknowledgement status = %d, want %d", unauthorizedResponse.Code, http.StatusUnauthorized)
	}
	if _, err := os.Stat(result); !os.IsNotExist(err) {
		t.Fatalf("unauthorized acknowledgement result = %v, want no result", err)
	}

	acknowledgement := httptest.NewRequest(http.MethodPost, "/ack", nil)
	acknowledgement.Header.Set("Authorization", "Bearer synthetic-token")
	acknowledgementResponse := httptest.NewRecorder()
	handler.ServeHTTP(acknowledgementResponse, acknowledgement)
	if acknowledgementResponse.Code != http.StatusNoContent {
		t.Fatalf("acknowledgement status = %d, want %d", acknowledgementResponse.Code, http.StatusNoContent)
	}

	contents, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "acknowledged\n" {
		t.Fatalf("acknowledgement contents = %q", contents)
	}
}
