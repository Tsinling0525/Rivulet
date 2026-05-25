package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientChatPostsMessagesAndParsesReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload struct {
			Model    string        `json:"model"`
			Messages []ChatMessage `json:"messages"`
			Stream   bool          `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "gemma4:e2b" {
			t.Fatalf("unexpected model: %s", payload.Model)
		}
		if payload.Stream {
			t.Fatal("expected non-streaming request")
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Content != "Hello" {
			t.Fatalf("unexpected messages: %#v", payload.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"gemma4:e2b",
			"message":{"role":"assistant","content":"Hi from Gemma"},
			"prompt_eval_count":4,
			"eval_count":8
		}`))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), ChatEndpoint: server.URL + "/api/chat"}
	completion, err := client.Chat(context.Background(), "gemma4:e2b", []ChatMessage{{Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatalf("chat returned error: %v", err)
	}
	if completion.Reply != "Hi from Gemma" {
		t.Fatalf("unexpected reply: %q", completion.Reply)
	}
	if completion.Usage.PromptEvalCount != 4 || completion.Usage.EvalCount != 8 {
		t.Fatalf("unexpected usage: %#v", completion.Usage)
	}
}
