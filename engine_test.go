package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestCallCPAManagementUsesBearerPasswordAndJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-management-password" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBaseURL := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() {
		cpaManagementBaseURL = oldBaseURL
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
	}()

	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-management-password")

	status, _, err := callCPAManagement(http.MethodPatch, "/status", []byte(`{"disabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
}
