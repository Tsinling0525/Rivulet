package httpnode

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"text/template"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

// HttpGet is a minimal HTTP GET node
// Config:
// - url: string (template supported with current item as data)
// - timeout: number (seconds, optional)
// - headers: map[string]string (optional)
type HttpGet struct{ deps plugin.Deps }

func (n *HttpGet) Init(ctx context.Context, deps plugin.Deps) error { n.deps = deps; return nil }

func renderTemplate(tpl string, data any) (string, error) {
	if tpl == "" {
		return "", nil
	}
	t, err := template.New("x").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytesBuffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// lightweight buffer to avoid importing bytes directly elsewhere
type bytesBuffer struct{ b []byte }

func (w *bytesBuffer) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *bytesBuffer) String() string              { return string(w.b) }

func (n *HttpGet) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	if v, ok := node.Config["timeout"].(float64); ok && v > 0 {
		client.Timeout = time.Duration(v * float64(time.Second))
	}

	var hdrs map[string]string
	if h, ok := node.Config["headers"].(map[string]any); ok {
		hdrs = make(map[string]string, len(h))
		for k, v := range h {
			if s, ok := v.(string); ok {
				hdrs[k] = s
			}
		}
	}

	urlTpl, _ := node.Config["url"].(string)
	out := make(model.Items, 0, len(in))
	for _, item := range in {
		if item == nil {
			item = model.Item{}
		}
		urlStr, err := renderTemplate(urlTpl, item)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range hdrs {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		func() { defer resp.Body.Close() }()
		bodyBytes, _ := io.ReadAll(resp.Body)

		// Try decode JSON; fallback to string
		var body any
		if json.Unmarshal(bodyBytes, &body) != nil {
			body = string(bodyBytes)
		}

		out = append(out, model.Item{
			"status":  resp.StatusCode,
			"body":    body,
			"url":     urlStr,
			"node_id": node.ID,
		})
	}
	return out, nil
}

func init() { plugin.Register("http:get", func() plugin.NodeHandler { return &HttpGet{} }) }
