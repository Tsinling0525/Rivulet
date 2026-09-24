package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Tsinling0525/rivulet/dsl"
	"github.com/Tsinling0525/rivulet/expr"
)

// HTTPRequest calls an external HTTP endpoint (Dify's http-request node).
type HTTPRequest struct{}

func (HTTPRequest) Type() string { return dsl.NodeHTTP }

const (
	httpDefaultTimeout = 30 * time.Second
	httpResponseLimit  = 4 << 20
)

type httpAuthConfig struct {
	Type   string `yaml:"type"`
	APIKey string `yaml:"api_key"`
	Header string `yaml:"header"`
	Value  string `yaml:"value"`
}

type httpAuthorization struct {
	Type   string         `yaml:"type"`
	Config httpAuthConfig `yaml:"config"`
}

type httpBody struct {
	Type string `yaml:"type"`
	Data any    `yaml:"data"`
}

type httpConfig struct {
	Method        string            `yaml:"method"`
	URL           string            `yaml:"url"`
	Headers       string            `yaml:"headers"`
	Params        string            `yaml:"params"`
	Body          httpBody          `yaml:"body"`
	Authorization httpAuthorization `yaml:"authorization"`
	Timeout       float64           `yaml:"timeout"`
}

var httpMethods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true, http.MethodHead: true,
}

func (HTTPRequest) Validate(node dsl.Node) []dsl.Problem {
	var cfg httpConfig
	if problems := decodeExtra(node, &cfg); problems != nil {
		return problems
	}
	var problems []dsl.Problem
	if strings.TrimSpace(cfg.URL) == "" {
		problems = append(problems, problem(node, "url must not be empty"))
	}
	if method := strings.ToUpper(strings.TrimSpace(cfg.Method)); method != "" && !httpMethods[method] {
		problems = append(problems, problem(node, "method %q is not supported", cfg.Method))
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Body.Type)) {
	case "", "none", "json", "raw", "form-data", "x-www-form-urlencoded":
	default:
		problems = append(problems, problem(node, "body.type %q is not supported", cfg.Body.Type))
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Authorization.Type)) {
	case "", "no-auth", "none", "api-key", "custom":
	default:
		problems = append(problems, problem(node, "authorization.type %q is not supported", cfg.Authorization.Type))
	}
	return problems
}

func (HTTPRequest) Run(ctx context.Context, req Request) (Output, error) {
	var cfg httpConfig
	if err := req.Node.Data.Decode(&cfg); err != nil {
		return Output{}, nodeError(req.Node, "invalid configuration: %v", err)
	}

	target, err := req.Render(cfg.URL)
	if err != nil {
		return Output{}, nodeError(req.Node, "url: %v", err)
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return Output{}, nodeError(req.Node, "url must not be empty")
	}

	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = http.MethodGet
	}

	body, contentType, err := buildHTTPBody(req, cfg)
	if err != nil {
		return Output{}, err
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return Output{}, nodeError(req.Node, "%v", err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if err := applyHTTPParams(req, request, cfg.Params); err != nil {
		return Output{}, err
	}
	if err := applyHTTPHeaders(req, request, cfg.Headers); err != nil {
		return Output{}, err
	}
	if err := applyHTTPAuthorization(req, request, cfg.Authorization); err != nil {
		return Output{}, err
	}

	client := req.HTTPClient
	if client == nil {
		timeout := httpDefaultTimeout
		if cfg.Timeout > 0 {
			timeout = time.Duration(cfg.Timeout * float64(time.Second))
		}
		client = &http.Client{Timeout: timeout}
	}

	req.Log("%s %s", method, target)
	response, err := client.Do(request)
	if err != nil {
		return Output{}, nodeError(req.Node, "%v", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, httpResponseLimit))
	if err != nil {
		return Output{}, nodeError(req.Node, "read response: %v", err)
	}
	if response.StatusCode >= 400 {
		return Output{}, nodeError(req.Node, "request failed with %s: %s",
			response.Status, truncateForError(strings.TrimSpace(string(raw))))
	}

	fields := map[string]any{
		"status_code": response.StatusCode,
		"headers":     flattenHeaders(response.Header),
		"body":        decodeHTTPBody(raw),
	}
	return Output{Fields: fields}, nil
}

func buildHTTPBody(req Request, cfg httpConfig) (io.Reader, string, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Body.Type)) {
	case "", "none":
		return nil, "", nil
	case "json":
		payload, err := renderJSONValue(req, cfg.Body.Data, "body.data")
		if err != nil {
			return nil, "", err
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, "", nodeError(req.Node, "encode json body: %v", err)
		}
		return bytes.NewReader(encoded), "application/json", nil
	case "raw":
		text, err := renderAnyString(req, cfg.Body.Data, "body.data")
		if err != nil {
			return nil, "", err
		}
		return strings.NewReader(text), "text/plain", nil
	case "x-www-form-urlencoded":
		values := url.Values{}
		if err := collectFormValues(req, cfg.Body.Data, func(key, value string) {
			values.Add(key, value)
		}); err != nil {
			return nil, "", err
		}
		return strings.NewReader(values.Encode()), "application/x-www-form-urlencoded", nil
	case "form-data":
		buffer := &bytes.Buffer{}
		writer := multipart.NewWriter(buffer)
		err := collectFormValues(req, cfg.Body.Data, func(key, value string) {
			_ = writer.WriteField(key, value)
		})
		if err != nil {
			return nil, "", err
		}
		if err := writer.Close(); err != nil {
			return nil, "", nodeError(req.Node, "encode form body: %v", err)
		}
		return buffer, writer.FormDataContentType(), nil
	default:
		return nil, "", nodeError(req.Node, "body.type %q is not supported", cfg.Body.Type)
	}
}

func collectFormValues(req Request, data any, add func(key, value string)) error {
	items, ok := data.([]any)
	if !ok {
		return nodeError(req.Node, "body.data must be a list of {key, value} pairs for this body type, got %T", data)
	}
	for index, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nodeError(req.Node, "body.data[%d] must be a mapping with key and value", index)
		}
		key, err := renderAnyString(req, entry["key"], fmt.Sprintf("body.data[%d].key", index))
		if err != nil {
			return err
		}
		value, err := renderAnyString(req, entry["value"], fmt.Sprintf("body.data[%d].value", index))
		if err != nil {
			return err
		}
		add(key, value)
	}
	return nil
}

// renderJSONValue walks a decoded body and renders every string leaf.
func renderJSONValue(req Request, value any, path string) (any, error) {
	switch v := value.(type) {
	case string:
		rendered, err := req.Render(v)
		if err != nil {
			return nil, nodeError(req.Node, "%s: %v", path, err)
		}
		return rendered, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			rendered, err := renderJSONValue(req, item, path+"."+key)
			if err != nil {
				return nil, err
			}
			out[key] = rendered
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(v))
		for index, item := range v {
			rendered, err := renderJSONValue(req, item, fmt.Sprintf("%s[%d]", path, index))
			if err != nil {
				return nil, err
			}
			out = append(out, rendered)
		}
		return out, nil
	default:
		return value, nil
	}
}

func renderAnyString(req Request, value any, path string) (string, error) {
	if value == nil {
		return "", nil
	}
	rendered, err := req.Render(expr.Stringify(value))
	if err != nil {
		return "", nodeError(req.Node, "%s: %v", path, err)
	}
	return rendered, nil
}

func applyHTTPParams(req Request, request *http.Request, params string) error {
	if strings.TrimSpace(params) == "" {
		return nil
	}
	query := request.URL.Query()
	for index, line := range splitLines(params) {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nodeError(req.Node, "params line %d is not key=value: %q", index+1, line)
		}
		rendered, err := req.Render(value)
		if err != nil {
			return nodeError(req.Node, "params line %d: %v", index+1, err)
		}
		query.Set(strings.TrimSpace(key), rendered)
	}
	request.URL.RawQuery = query.Encode()
	return nil
}

func applyHTTPHeaders(req Request, request *http.Request, headers string) error {
	if strings.TrimSpace(headers) == "" {
		return nil
	}
	for index, line := range splitLines(headers) {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nodeError(req.Node, "headers line %d is not \"Name: value\": %q", index+1, line)
		}
		rendered, err := req.Render(value)
		if err != nil {
			return nodeError(req.Node, "headers line %d: %v", index+1, err)
		}
		request.Header.Set(strings.TrimSpace(name), strings.TrimSpace(rendered))
	}
	return nil
}

func applyHTTPAuthorization(req Request, request *http.Request, auth httpAuthorization) error {
	switch strings.ToLower(strings.TrimSpace(auth.Type)) {
	case "", "no-auth", "none":
		return nil
	case "api-key":
		apiKey, err := req.Render(auth.Config.APIKey)
		if err != nil {
			return nodeError(req.Node, "authorization.config.api_key: %v", err)
		}
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			// An unset key resolves to an empty string; send the request without
			// credentials instead of an "Authorization: Bearer " header.
			req.Log("authorization.api_key resolved to empty; sending the request without credentials")
			return nil
		}
		header := strings.TrimSpace(auth.Config.Header)
		if header == "" {
			header = "Authorization"
		}
		switch strings.ToLower(strings.TrimSpace(auth.Config.Type)) {
		case "basic":
			request.Header.Set(header, "Basic "+apiKey)
		default:
			request.Header.Set(header, "Bearer "+apiKey)
		}
		return nil
	case "custom":
		header := strings.TrimSpace(auth.Config.Header)
		if header == "" {
			return nodeError(req.Node, "authorization.config.header is required for custom auth")
		}
		value, err := req.Render(auth.Config.Value)
		if err != nil {
			return nodeError(req.Node, "authorization.config.value: %v", err)
		}
		request.Header.Set(header, value)
		return nil
	default:
		return nodeError(req.Node, "authorization.type %q is not supported", auth.Type)
	}
}

func flattenHeaders(header http.Header) map[string]string {
	out := make(map[string]string, len(header))
	for name, values := range header {
		out[name] = strings.Join(values, ", ")
	}
	return out
}

func decodeHTTPBody(raw []byte) any {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err == nil {
		return decoded
	}
	return text
}

func splitLines(text string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func truncateForError(text string) string {
	const limit = 400
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
