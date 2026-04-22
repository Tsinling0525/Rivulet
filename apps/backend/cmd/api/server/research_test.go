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

func TestHandleAgenticPaperSearch(t *testing.T) {
	var arxivQuery string
	arxivServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arxivQuery = r.URL.Query().Get("search_query")
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>https://arxiv.org/abs/2601.00001</id>
    <title>Agentic Machine Learning Systems</title>
    <summary>
      We study agentic machine learning systems that plan, call tools, and evaluate intermediate results.
    </summary>
    <published>2026-01-01T00:00:00Z</published>
    <author><name>Ada Lovelace</name></author>
    <link href="https://arxiv.org/abs/2601.00001" rel="alternate" type="text/html"/>
  </entry>
</feed>`))
	}))
	defer arxivServer.Close()

	var ollamaPayload struct {
		Model    string               `json:"model"`
		Messages []ollama.ChatMessage `json:"messages"`
	}
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&ollamaPayload); err != nil {
			t.Fatalf("decode ollama request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gemma4:e2b","message":{"role":"assistant","content":"总结：这些论文关注 agentic ML 的规划、工具调用和评估。"},"prompt_eval_count":10,"eval_count":20}`))
	}))
	defer ollamaServer.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/research/agentic-papers", handleAgenticPaperSearch(arxivServer.URL, arxivServer.Client(), &ollama.Client{
		HTTPClient:   ollamaServer.Client(),
		ChatEndpoint: ollamaServer.URL,
	}))

	req := httptest.NewRequest(http.MethodPost, "/research/agentic-papers", strings.NewReader(`{
		"query":"agentic machine learning",
		"limit":1
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(arxivQuery, "agentic machine learning") {
		t.Fatalf("unexpected arxiv query: %q", arxivQuery)
	}
	if ollamaPayload.Model != "gemma4:e2b" {
		t.Fatalf("unexpected model: %s", ollamaPayload.Model)
	}
	if len(ollamaPayload.Messages) != 2 || !strings.Contains(ollamaPayload.Messages[1].Content, "Agentic Machine Learning Systems") {
		t.Fatalf("unexpected ollama messages: %#v", ollamaPayload.Messages)
	}

	var response APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success {
		t.Fatalf("unexpected failure: %#v", response)
	}
	if !strings.Contains(response.Data["summary"].(string), "agentic ML") {
		t.Fatalf("unexpected summary: %#v", response.Data["summary"])
	}
	papers := response.Data["papers"].([]any)
	if len(papers) != 1 {
		t.Fatalf("unexpected papers: %#v", papers)
	}
}
