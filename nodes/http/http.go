package httpnode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type HTTP struct {
	deps plugin.Deps
	cl   *http.Client
}

func (n *HTTP) Init(ctx context.Context, deps plugin.Deps) error {
	n.deps = deps
	n.cl = &http.Client{Timeout: 15 * time.Second}
	return nil
}

func (n *HTTP) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	method, _ := node.Config["method"].(string)
	urlT, _ := node.Config["url"].(string)
	bodyKey, _ := node.Config["bodyKey"].(string)
	retries, _ := node.Config["retries"].(int)

	out := make(model.Items, 0, len(in))
	for _, it := range in {
		url := urlT
		var body io.Reader
		if b, ok := it[bodyKey]; ok && b != nil {
			p, _ := json.Marshal(b)
			body = bytes.NewReader(p)
		}
		var respObj map[string]any
		var lastErr error
		for attempt := 0; attempt <= retries; attempt++ {
			req, _ := http.NewRequestWithContext(ctx, method, url, body)
			req.Header.Set("Content-Type", "application/json")
			res, err := n.cl.Do(req)
			if err != nil {
				lastErr = err
				time.Sleep(backoff(attempt))
				continue
			}
			defer res.Body.Close()
			if res.StatusCode >= 500 && res.StatusCode < 600 {
				lastErr = fmt.Errorf("server error status: %d", res.StatusCode)
				time.Sleep(backoff(attempt))
				continue
			}
			if err := json.NewDecoder(res.Body).Decode(&respObj); err != nil {
				respObj = map[string]any{"status": res.StatusCode}
			}
			break
		}
		if lastErr != nil {
			out = append(out, model.Item{"error": lastErr.Error()})
		} else {
			out = append(out, model.Item{"response": respObj})
		}
	}
	return out, nil
}

func backoff(i int) time.Duration {
	if i == 0 {
		return 200 * time.Millisecond
	}
	d := 200 * time.Millisecond
	for k := 0; k < i; k++ {
		d *= 2
	}
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

func init() { plugin.Register("http", func() plugin.NodeHandler { return &HTTP{} }) }
