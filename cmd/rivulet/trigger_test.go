package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSendTriggerPostsJSONBodyWithHeaders(t *testing.T) {
	var gotMethod, gotContentType, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"queued"}`))
	}))
	defer server.Close()

	var out strings.Builder
	err := sendTrigger(context.Background(), server.Client(), triggerOptions{
		URL:     server.URL + "/webhook/run",
		Body:    `{"topic":"rivulet"}`,
		Headers: []string{"Authorization: Bearer app-test"},
	}, &out)
	if err != nil {
		t.Fatalf("sendTrigger: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("content type = %q, want application/json", gotContentType)
	}
	if gotAuth != "Bearer app-test" {
		t.Errorf("authorization = %q, want Bearer app-test", gotAuth)
	}
	if gotBody != `{"topic":"rivulet"}` {
		t.Errorf("body = %q, want the literal payload", gotBody)
	}
	if !strings.Contains(out.String(), "200 OK") || !strings.Contains(out.String(), `{"status":"queued"}`) {
		t.Errorf("output = %q, want status line and response body", out.String())
	}
}

func TestSendTriggerUsesGETWhenNoBody(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var out strings.Builder
	if err := sendTrigger(context.Background(), server.Client(), triggerOptions{URL: server.URL}, &out); err != nil {
		t.Fatalf("sendTrigger: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if !strings.Contains(out.String(), "204 No Content") {
		t.Errorf("output = %q, want the status line", out.String())
	}
}

func TestSendTriggerFailsOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "workflow not found", http.StatusNotFound)
	}))
	defer server.Close()

	var out strings.Builder
	err := sendTrigger(context.Background(), server.Client(), triggerOptions{URL: server.URL, Body: "{}"}, &out)
	if err == nil {
		t.Fatal("sendTrigger returned nil error for a 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want the HTTP status", err)
	}
	if !strings.Contains(out.String(), "workflow not found") {
		t.Errorf("output = %q, want the error body printed for the caller", out.String())
	}
}

func TestSendTriggerRejectsMalformedHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not be sent with a malformed header")
	}))
	defer server.Close()

	var out strings.Builder
	err := sendTrigger(context.Background(), server.Client(), triggerOptions{URL: server.URL, Headers: []string{"nonsense"}}, &out)
	if err == nil || !strings.Contains(err.Error(), "invalid --header") {
		t.Fatalf("error = %v, want an invalid --header error", err)
	}
}

func TestResolveTriggerBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(path, []byte(`{"from":"file"}`), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	tests := []struct {
		name    string
		data    string
		file    string
		stdin   string
		want    string
		wantErr string
	}{
		{name: "literal data", data: `{"a":1}`, want: `{"a":1}`},
		{name: "empty body", want: ""},
		{name: "file", file: path, want: `{"from":"file"}`},
		{name: "stdin", file: "-", stdin: `{"from":"stdin"}`, want: `{"from":"stdin"}`},
		{name: "both sources", data: `{}`, file: path, wantErr: "not both"},
		{name: "missing file", file: filepath.Join(t.TempDir(), "absent.json"), wantErr: "read body file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveTriggerBody(tc.data, tc.file, strings.NewReader(tc.stdin))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTriggerBody: %v", err)
			}
			if got != tc.want {
				t.Fatalf("body = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunTriggerCLIRequiresURL(t *testing.T) {
	t.Setenv("RIVULET_TRIGGER_URL", "")
	err := runTriggerCLI([]string{"--data", "{}"})
	if err == nil || !strings.Contains(err.Error(), "--url is required") {
		t.Fatalf("error = %v, want a missing --url error", err)
	}
}

func TestRunTriggerCLIUsesEnvironmentURL(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	t.Setenv("RIVULET_TRIGGER_URL", server.URL+"/webhook/env")
	if err := runTriggerCLI([]string{"--timeout", "5"}); err != nil {
		t.Fatalf("runTriggerCLI: %v", err)
	}
	if gotPath != "/webhook/env" {
		t.Fatalf("path = %q, want /webhook/env", gotPath)
	}
}

func TestRunTriggerCLIRejectsNonPositiveTimeout(t *testing.T) {
	err := runTriggerCLI([]string{"--url", "http://127.0.0.1:1/none", "--timeout", "0"})
	if err == nil || !strings.Contains(err.Error(), "--timeout must be positive") {
		t.Fatalf("error = %v, want a timeout validation error", err)
	}
}

func TestSendTriggerHonoursContextTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var out strings.Builder
	err := sendTrigger(ctx, server.Client(), triggerOptions{URL: server.URL}, &out)
	if err == nil {
		t.Fatal("sendTrigger returned nil error for a cancelled context")
	}
}
