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


func TestResolveManagementPasswordPrefersRequestBearer(t *testing.T) {
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword) }()
	_ = os.Setenv("MANAGEMENT_PASSWORD", "env-password")

	headers := http.Header{"Authorization": []string{"Bearer page-password"}}
	if got := resolveManagementPassword(headers); got != "page-password" {
		t.Fatalf("password = %q, want page-password", got)
	}
	if got := resolveManagementPassword(nil); got != "env-password" {
		t.Fatalf("env password = %q, want env-password", got)
	}
}

func TestCallCPAManagementWithAuthUsesRequestPasswordWithoutEnv(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer page-password" {
			t.Fatalf("authorization = %q", got)
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
	_ = os.Unsetenv("MANAGEMENT_PASSWORD")
	_ = os.Unsetenv("CPA_MANAGEMENT_KEY")

	status, _, err := callCPAManagementWithAuth(http.MethodPatch, "/status", []byte(`{"disabled":true}`), "page-password", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
}

func TestResolveManagementBaseURLIgnoresRequestHostPort(t *testing.T) {
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := cpaManagementBaseURL
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		cpaManagementBaseURL = oldDefault
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	cpaManagementBaseURL = "http://127.0.0.1:8317"

	headers := http.Header{"Host": []string{"cpa.example.com:1109"}}
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:8317" {
		t.Fatalf("base url = %q, want default local management port", got)
	}

	_ = os.Setenv("CPA_BASE_URL", "http://127.0.0.1:9999")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:9999" {
		t.Fatalf("env base url = %q", got)
	}
}
