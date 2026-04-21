package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tsinling0525/rivulet/nodes/ollama"
)

func TestHandleOllamaChat(t *testing.T) {
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model    string               `json:"model"`
			Messages []ollama.ChatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "gemma4:e2b" {
			t.Fatalf("unexpected model: %s", payload.Model)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
			t.Fatalf("unexpected messages: %#v", payload.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gemma4:e2b","message":{"role":"assistant","content":"hello browser"}}`))
	}))
	defer ollamaServer.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/chat/ollama", handleOllamaChat(&ollama.Client{
		HTTPClient:   ollamaServer.Client(),
		ChatEndpoint: ollamaServer.URL,
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/chat/ollama", strings.NewReader(`{
		"model":"gemma4:e2b",
		"messages":[{"role":"user","content":"Hello"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}
	var response APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || response.Data["reply"] != "hello browser" {
		t.Fatalf("unexpected response: %#v", response)
	}
}
