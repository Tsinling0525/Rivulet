package server

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tsinling0525/rivulet/nodes/ollama"
)

const (
	defaultAgenticPaperQuery = `agentic AI agents machine learning`
	defaultArxivEndpoint     = "https://export.arxiv.org/api/query"
	maxAgenticPaperLimit     = 10
)

type arxivPaper struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	Authors   []string `json:"authors"`
	Published string   `json:"published"`
	Link      string   `json:"link"`
}

type agenticPaperRequest struct {
	Query  string       `json:"query"`
	Limit  int          `json:"limit"`
	Model  string       `json:"model"`
	Papers []arxivPaper `json:"papers"`
}

type researchTraceStep struct {
	ID         string         `json:"id"`
	Tool       string         `json:"tool"`
	Status     string         `json:"status"`
	Detail     string         `json:"detail"`
	StartedAt  string         `json:"started_at"`
	FinishedAt string         `json:"finished_at,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type arxivFeed struct {
	Entries []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	ID        string        `xml:"id"`
	Title     string        `xml:"title"`
	Summary   string        `xml:"summary"`
	Published string        `xml:"published"`
	Authors   []arxivAuthor `xml:"author"`
	Links     []arxivLink   `xml:"link"`
}

type arxivAuthor struct {
	Name string `xml:"name"`
}

type arxivLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

func handleAgenticPaperSearch(arxivEndpoint string, httpClient *http.Client, ollamaClient *ollama.Client) gin.HandlerFunc {
	deps := newResearchDeps(arxivEndpoint, httpClient, ollamaClient)
	return func(c *gin.Context) {
		payload, err := bindAgenticPaperRequest(c, false)
		if err != nil {
			sendError(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		query, limit, model := normalizeAgenticPaperRequest(payload)
		trace := []researchTraceStep{}

		papers, searchStep, err := runArxivSearch(c.Request.Context(), deps.HTTPClient, deps.ArxivEndpoint, query, limit)
		trace = append(trace, searchStep)
		if err != nil {
			sendError(c, http.StatusBadGateway, err.Error())
			return
		}
		if len(papers) == 0 {
			sendSuccess(c, map[string]any{
				"query":   query,
				"model":   model,
				"papers":  papers,
				"summary": "没有在 arXiv 检索到匹配论文。可以尝试放宽关键词，例如 language agents 或 autonomous agents。",
				"trace":   trace,
			})
			return
		}

		completion, summaryStep, err := summarizeAgenticPapers(c.Request.Context(), deps.OllamaClient, model, query, papers)
		trace = append(trace, summaryStep)
		if err != nil {
			sendError(c, http.StatusBadGateway, err.Error())
			return
		}

		sendSuccess(c, map[string]any{
			"query":             query,
			"model":             completion.Model,
			"papers":            papers,
			"summary":           strings.TrimSpace(completion.Reply),
			"reasoning_summary": visibleReasoningSummary(papers),
			"usage":             completion.Usage,
			"trace":             trace,
			"generated_at":      time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleAgenticPaperArxivSearch(arxivEndpoint string, httpClient *http.Client) gin.HandlerFunc {
	deps := newResearchDeps(arxivEndpoint, httpClient, nil)
	return func(c *gin.Context) {
		payload, err := bindAgenticPaperRequest(c, false)
		if err != nil {
			sendError(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		query, limit, model := normalizeAgenticPaperRequest(payload)
		papers, step, err := runArxivSearch(c.Request.Context(), deps.HTTPClient, deps.ArxivEndpoint, query, limit)
		if err != nil {
			sendError(c, http.StatusBadGateway, err.Error())
			return
		}
		sendSuccess(c, map[string]any{
			"query":  query,
			"model":  model,
			"papers": papers,
			"trace":  []researchTraceStep{step},
		})
	}
}

func handleAgenticPaperSummary(ollamaClient *ollama.Client) gin.HandlerFunc {
	deps := newResearchDeps("", nil, ollamaClient)
	return func(c *gin.Context) {
		payload, err := bindAgenticPaperRequest(c, true)
		if err != nil {
			sendError(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		query, _, model := normalizeAgenticPaperRequest(payload)
		if len(payload.Papers) == 0 {
			sendError(c, http.StatusBadRequest, "papers must contain at least one paper")
			return
		}

		completion, step, err := summarizeAgenticPapers(c.Request.Context(), deps.OllamaClient, model, query, payload.Papers)
		if err != nil {
			sendError(c, http.StatusBadGateway, err.Error())
			return
		}
		sendSuccess(c, map[string]any{
			"query":             query,
			"model":             completion.Model,
			"papers":            payload.Papers,
			"summary":           strings.TrimSpace(completion.Reply),
			"reasoning_summary": visibleReasoningSummary(payload.Papers),
			"usage":             completion.Usage,
			"trace":             []researchTraceStep{step},
			"generated_at":      time.Now().UTC().Format(time.RFC3339),
		})
	}
}

type researchDeps struct {
	ArxivEndpoint string
	HTTPClient    *http.Client
	OllamaClient  *ollama.Client
}

func newResearchDeps(arxivEndpoint string, httpClient *http.Client, ollamaClient *ollama.Client) researchDeps {
	if strings.TrimSpace(arxivEndpoint) == "" {
		arxivEndpoint = defaultArxivEndpoint
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	if ollamaClient == nil {
		ollamaClient = &ollama.Client{HTTPClient: &http.Client{Timeout: 3 * time.Minute}}
	}
	return researchDeps{
		ArxivEndpoint: arxivEndpoint,
		HTTPClient:    httpClient,
		OllamaClient:  ollamaClient,
	}
}

func bindAgenticPaperRequest(c *gin.Context, requireBody bool) (agenticPaperRequest, error) {
	var payload agenticPaperRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		if !requireBody && errors.Is(err, io.EOF) {
			return payload, nil
		}
		return payload, err
	}
	return payload, nil
}

func normalizeAgenticPaperRequest(payload agenticPaperRequest) (string, int, string) {
	query := strings.TrimSpace(payload.Query)
	if query == "" {
		query = defaultAgenticPaperQuery
	}
	limit := payload.Limit
	if limit <= 0 {
		limit = 1
	}
	if limit > maxAgenticPaperLimit {
		limit = maxAgenticPaperLimit
	}
	model := strings.TrimSpace(payload.Model)
	if model == "" {
		model = ollama.DefaultModel
	}
	return query, limit, model
}

func runArxivSearch(ctx context.Context, client *http.Client, endpoint, query string, limit int) ([]arxivPaper, researchTraceStep, error) {
	start := time.Now()
	step := researchTraceStep{
		ID:        "arxiv_search",
		Tool:      "arXiv API",
		Status:    "running",
		Detail:    "Search arXiv for recent ML/AI papers matching the query.",
		StartedAt: start.UTC().Format(time.RFC3339),
		Metadata: map[string]any{
			"query": query,
			"limit": limit,
		},
	}
	papers, err := searchArxiv(ctx, client, endpoint, query, limit)
	finishTraceStep(&step, start, err == nil)
	step.Metadata["paper_count"] = len(papers)
	if err != nil {
		step.Detail = err.Error()
	}
	return papers, step, err
}

func summarizeAgenticPapers(ctx context.Context, client *ollama.Client, model, query string, papers []arxivPaper) (ollama.ChatCompletion, researchTraceStep, error) {
	start := time.Now()
	step := researchTraceStep{
		ID:        "gemma_summary",
		Tool:      "Ollama Gemma4",
		Status:    "running",
		Detail:    "Summarize retrieved titles and abstracts with the local model.",
		StartedAt: start.UTC().Format(time.RFC3339),
		Metadata: map[string]any{
			"model":       model,
			"paper_count": len(papers),
		},
	}
	completion, err := client.Chat(ctx, model, []ollama.ChatMessage{
		{
			Role:    "system",
			Content: "你是机器学习论文研究助手。只基于用户提供的 arXiv 论文标题和摘要总结，不要编造未提供的论文或结论。不要输出隐藏思维链；只输出可展示的结论、依据和分析摘要。",
		},
		{
			Role:    "user",
			Content: buildAgenticPaperPrompt(query, papers),
		},
	})
	finishTraceStep(&step, start, err == nil)
	if err != nil {
		step.Detail = err.Error()
		return completion, step, err
	}
	step.Metadata["prompt_eval_count"] = completion.Usage.PromptEvalCount
	step.Metadata["eval_count"] = completion.Usage.EvalCount
	return completion, step, nil
}

func finishTraceStep(step *researchTraceStep, start time.Time, success bool) {
	step.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	step.DurationMS = time.Since(start).Milliseconds()
	if success {
		step.Status = "completed"
	} else {
		step.Status = "failed"
	}
}

func visibleReasoningSummary(papers []arxivPaper) []string {
	return []string{
		fmt.Sprintf("检索阶段从 arXiv 取回 %d 篇候选论文，并把标题、作者、发布时间和摘要作为后续输入。", len(papers)),
		"总结阶段只把这些检索结果交给本地 Gemma4，不要求模型凭空补充论文。",
		"前端展示的是可审计的工具链和可展示分析摘要，不包含模型隐藏推理链。",
	}
}

func searchArxiv(ctx context.Context, client *http.Client, endpoint, query string, limit int) ([]arxivPaper, error) {
	values := url.Values{}
	values.Set("search_query", arxivSearchQuery(query))
	values.Set("start", "0")
	values.Set("max_results", strconv.Itoa(limit))
	values.Set("sortBy", "submittedDate")
	values.Set("sortOrder", "descending")

	reqURL := endpoint + "?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Rivulet/1.0 paper research")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("arXiv search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("arXiv search failed: status %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var feed arxivFeed
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decode arXiv response: %w", err)
	}

	papers := make([]arxivPaper, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		paper := arxivPaper{
			ID:        strings.TrimSpace(entry.ID),
			Title:     compactWhitespace(entry.Title),
			Summary:   compactWhitespace(entry.Summary),
			Authors:   arxivAuthors(entry.Authors),
			Published: strings.TrimSpace(entry.Published),
			Link:      arxivEntryLink(entry),
		}
		if paper.Title != "" {
			papers = append(papers, paper)
		}
	}
	return papers, nil
}

func arxivSearchQuery(query string) string {
	terms := strings.TrimSpace(query)
	if terms == "" {
		terms = defaultAgenticPaperQuery
	}
	if strings.EqualFold(terms, defaultAgenticPaperQuery) {
		return `(all:agentic OR all:"AI agents" OR all:"language agents" OR all:"autonomous agents" OR all:"LLM agents") AND (cat:cs.LG OR cat:cs.AI OR cat:cs.CL OR cat:stat.ML)`
	}
	return fmt.Sprintf(`(all:"%s") AND (cat:cs.LG OR cat:cs.AI OR cat:cs.CL OR cat:stat.ML)`, strings.ReplaceAll(terms, `"`, " "))
}

func arxivAuthors(authors []arxivAuthor) []string {
	out := make([]string, 0, len(authors))
	for _, author := range authors {
		name := compactWhitespace(author.Name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func arxivEntryLink(entry arxivEntry) string {
	for _, link := range entry.Links {
		if link.Rel == "alternate" && link.Href != "" {
			return link.Href
		}
	}
	for _, link := range entry.Links {
		if link.Href != "" {
			return link.Href
		}
	}
	return strings.TrimSpace(entry.ID)
}

func buildAgenticPaperPrompt(query string, papers []arxivPaper) string {
	var b strings.Builder
	b.WriteString("请用中文总结以下 arXiv 检索结果。\n")
	b.WriteString("输出结构：\n")
	b.WriteString("1. 总体结论：2-3 句。\n")
	b.WriteString("2. 代表论文：逐篇列出标题、核心问题、方法或贡献、值得关注的局限。\n")
	b.WriteString("3. 主题归纳：总结这些论文反映出的 agentic ML 研究方向。\n")
	b.WriteString("4. 下一步阅读建议：给出 3 条具体建议。\n\n")
	b.WriteString("检索词：")
	b.WriteString(query)
	b.WriteString("\n\n论文：\n")
	for i, paper := range papers {
		fmt.Fprintf(&b, "%d. Title: %s\n", i+1, paper.Title)
		if len(paper.Authors) > 0 {
			fmt.Fprintf(&b, "Authors: %s\n", strings.Join(paper.Authors, ", "))
		}
		if paper.Published != "" {
			fmt.Fprintf(&b, "Published: %s\n", paper.Published)
		}
		fmt.Fprintf(&b, "Abstract: %s\n\n", paper.Summary)
	}
	return b.String()
}

func compactWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
