package llmclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatSendsChatCompletionsShape(t *testing.T) {
	var got map[string]any
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi there"}}],"usage":{"total_tokens":9}}`))
	}))
	defer server.Close()

	temperature := 0.25
	result, err := Chat(context.Background(), Config{
		Endpoint:        server.URL + "/chat/completions",
		APIKey:          "secret",
		Model:           "m",
		MaxOutputTokens: 123,
		ResponseFormat:  "json_object",
		Temperature:     &temperature,
		ExtraFields:     map[string]any{"top_p": 0.9},
	}, []Message{{Role: "system", Content: "be terse"}, {Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if result.Text != "hi there" {
		t.Fatalf("text = %q", result.Text)
	}
	if result.Usage["total_tokens"] != float64(9) {
		t.Fatalf("usage = %#v", result.Usage)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if got["max_tokens"] != float64(123) || got["temperature"] != 0.25 || got["top_p"] != 0.9 {
		t.Fatalf("payload = %#v", got)
	}
	format, _ := got["response_format"].(map[string]any)
	if format["type"] != "json_object" {
		t.Fatalf("response_format = %#v", got["response_format"])
	}
	messages, _ := got["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", got["messages"])
	}
	if _, hasInput := got["input"]; hasInput {
		t.Fatalf("chat shape must not send input: %#v", got)
	}
}

func TestChatSendsResponsesShape(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(`{"output":[{"content":[{"type":"output_text","text":"from responses"}]}]}`))
	}))
	defer server.Close()

	result, err := Chat(context.Background(), Config{
		Endpoint:        server.URL + "/responses",
		APIKey:          "secret",
		Model:           "m",
		MaxOutputTokens: 50,
	}, []Message{{Role: "system", Content: "part one"}, {Role: "user", Content: "part two"}})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if result.Text != "from responses" {
		t.Fatalf("text = %q", result.Text)
	}
	if got["input"] != "part one\n\npart two" {
		t.Fatalf("input = %#v", got["input"])
	}
	if got["max_output_tokens"] != float64(50) {
		t.Fatalf("payload = %#v", got)
	}
	if _, hasMessages := got["messages"]; hasMessages {
		t.Fatalf("responses shape must not send messages: %#v", got)
	}
}

func TestChatPrefersOutputText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"output_text":"top level","output":[{"content":[{"type":"output_text","text":"nested"}]}]}`))
	}))
	defer server.Close()

	result, err := Chat(context.Background(), Config{Endpoint: server.URL + "/responses", APIKey: "k", Model: "m"},
		[]Message{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if result.Text != "top level" {
		t.Fatalf("text = %q", result.Text)
	}
}

func TestChatRequiresCredentialsAndMessages(t *testing.T) {
	if _, err := Chat(context.Background(), Config{Endpoint: "http://127.0.0.1:1/x"}, []Message{{Role: "user", Content: "x"}}); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("error = %v, want ErrMissingAPIKey", err)
	}
	if _, err := Chat(context.Background(), Config{Endpoint: "http://127.0.0.1:1/x", APIKey: "k"}, nil); err == nil {
		t.Fatal("expected an error for an empty message list")
	}
}

func TestChatReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := Chat(context.Background(), Config{Endpoint: server.URL + "/chat/completions", APIKey: "k", Model: "m"},
		[]Message{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("error = %v", err)
	}
}

func TestChatRejectsMalformedResponses(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		body     string
		want     string
	}{
		{name: "no choices", endpoint: "/chat/completions", body: `{"choices":[]}`, want: "no choices"},
		{name: "invalid json", endpoint: "/chat/completions", body: `{`, want: "unexpected end of JSON"},
		{name: "no output text", endpoint: "/responses", body: `{"output":[]}`, want: "no output text"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			_, err := Chat(context.Background(), Config{Endpoint: server.URL + tc.endpoint, APIKey: "k", Model: "m"},
				[]Message{{Role: "user", Content: "x"}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestCompleteUsesSingleUserMessage(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer server.Close()

	text, err := Complete(context.Background(), Config{
		Endpoint: server.URL + "/chat/completions",
		APIKey:   "k",
		Model:    "m",
	}, "the prompt")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if text != "done" {
		t.Fatalf("text = %q", text)
	}
	messages, _ := got["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	first, _ := messages[0].(map[string]any)
	if first["role"] != "user" || first["content"] != "the prompt" {
		t.Fatalf("message = %#v", first)
	}
	if got["max_tokens"] != float64(defaultMaxTokens) {
		t.Fatalf("default max_tokens = %#v", got["max_tokens"])
	}
}

func TestJoinMessages(t *testing.T) {
	if got := joinMessages([]Message{{Role: "user", Content: "only"}}); got != "only" {
		t.Fatalf("joinMessages = %q", got)
	}
	if got := joinMessages(nil); got != "" {
		t.Fatalf("joinMessages(nil) = %q", got)
	}
}
