package httpnode

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "strings"
    "text/template"
    "time"

    "github.com/Tsinling0525/rivulet/model"
    "github.com/Tsinling0525/rivulet/plugin"
)

// HttpRequest supports JSON or multipart POST/PUT with simple polling.
// Config:
// - method: string (default: POST)
// - url: string (template)
// - headers: map[string]string
// - json_body: map[string]any (templated strings inside)
// - multipart_file_field: string (e.g., "file"); if set, sends multipart/form-data
// - file_bytes_field: string (default: "file_bytes")
// - file_name_field: string (default: "file_name")
// - timeout: number seconds (default 60)
// - poll: { enabled: bool, url: string (template), interval_ms: int, max_attempts: int, done_expr: string(template on last body to "true"/"false") }
type HttpRequest struct{ deps plugin.Deps }

func (n *HttpRequest) Init(ctx context.Context, deps plugin.Deps) error { n.deps = deps; return nil }

func (n *HttpRequest) render(tpl string, data any) (string, error) {
    if tpl == "" { return "", nil }
    t, err := template.New("x").Parse(tpl)
    if err != nil { return "", err }
    var buf bytes.Buffer
    if err := t.Execute(&buf, data); err != nil { return "", err }
    return buf.String(), nil
}

func (n *HttpRequest) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
    method, _ := node.Config["method"].(string)
    if method == "" { method = http.MethodPost }
    urlTpl, _ := node.Config["url"].(string)
    timeout := 60 * time.Second
    if v, ok := node.Config["timeout"].(float64); ok && v > 0 { timeout = time.Duration(v * float64(time.Second)) }
    client := &http.Client{Timeout: timeout}

    // headers
    var hdrs map[string]string
    if h, ok := node.Config["headers"].(map[string]any); ok {
        hdrs = make(map[string]string, len(h))
        for k, v := range h { if s, ok := v.(string); ok { hdrs[k] = s } }
    }

    // body config
    var jsonBody map[string]any
    if jb, ok := node.Config["json_body"].(map[string]any); ok {
        jsonBody = jb
    }
    fileField, _ := node.Config["multipart_file_field"].(string)
    fileBytesField, _ := node.Config["file_bytes_field"].(string)
    if fileBytesField == "" { fileBytesField = "file_bytes" }
    fileNameField, _ := node.Config["file_name_field"].(string)
    if fileNameField == "" { fileNameField = "file_name" }

    // poll config
    var poll struct {
        Enabled     bool
        URL         string
        IntervalMS  int
        MaxAttempts int
        DoneExpr    string
    }
    if p, ok := node.Config["poll"].(map[string]any); ok {
        if b, ok := p["enabled"].(bool); ok { poll.Enabled = b }
        if s, ok := p["url"].(string); ok { poll.URL = s }
        if i, ok := p["interval_ms"].(float64); ok { poll.IntervalMS = int(i) }
        if i, ok := p["max_attempts"].(float64); ok { poll.MaxAttempts = int(i) }
        if s, ok := p["done_expr"].(string); ok { poll.DoneExpr = s }
        if poll.IntervalMS <= 0 { poll.IntervalMS = 1000 }
        if poll.MaxAttempts <= 0 { poll.MaxAttempts = 60 }
    }

    out := make(model.Items, 0, len(in))
    for _, item := range in {
        if item == nil { item = model.Item{} }
        urlStr, err := n.render(urlTpl, item)
        if err != nil { return nil, err }

        var req *http.Request
        if fileField != "" {
            // multipart upload
            var b bytes.Buffer
            mw := multipart.NewWriter(&b)
            // file part
            fname, _ := item[fileNameField].(string)
            fbytes, _ := item[fileBytesField].([]byte)
            if fname == "" { fname = "file.bin" }
            fw, err := mw.CreateFormFile(fileField, fname)
            if err != nil { return nil, err }
            if _, err := fw.Write(fbytes); err != nil { return nil, err }
            // additional fields from json_body as strings
            for k, v := range jsonBody {
                var sval string
                switch vv := v.(type) {
                case string:
                    sval, err = n.render(vv, item)
                    if err != nil { return nil, err }
                default:
                    sval = fmt.Sprintf("%v", vv)
                }
                _ = mw.WriteField(k, sval)
            }
            _ = mw.Close()
            req, err = http.NewRequestWithContext(ctx, method, urlStr, &b)
            if err != nil { return nil, err }
            req.Header.Set("Content-Type", mw.FormDataContentType())
        } else if jsonBody != nil {
            // JSON body with templated strings
            bodyCopy := make(map[string]any, len(jsonBody))
            for k, v := range jsonBody {
                switch vv := v.(type) {
                case string:
                    sv, err := n.render(vv, item)
                    if err != nil { return nil, err }
                    bodyCopy[k] = sv
                default:
                    bodyCopy[k] = vv
                }
            }
            data, _ := json.Marshal(bodyCopy)
            req, err = http.NewRequestWithContext(ctx, method, urlStr, bytes.NewReader(data))
            if err != nil { return nil, err }
            req.Header.Set("Content-Type", "application/json")
        } else {
            // no body
            var err error
            req, err = http.NewRequestWithContext(ctx, method, urlStr, nil)
            if err != nil { return nil, err }
        }
        for k, v := range hdrs { req.Header.Set(k, v) }

        // perform request
        resp, err := client.Do(req)
        if err != nil { return nil, err }
        func() { defer resp.Body.Close() }()
        bodyBytes, _ := io.ReadAll(resp.Body)
        var body any
        if json.Unmarshal(bodyBytes, &body) != nil { body = string(bodyBytes) }

        // optional polling
        lastBody := body
        if poll.Enabled {
            done := func(val any) (bool, error) {
                if poll.DoneExpr == "" { return true, nil }
                t, err := template.New("done").Parse(poll.DoneExpr)
                if err != nil { return false, err }
                var buf bytes.Buffer
                if err := t.Execute(&buf, val); err != nil { return false, err }
                return strings.TrimSpace(buf.String()) == "true", nil
            }
            ok, err := done(lastBody)
            if err != nil { return nil, err }
            attempts := 0
            for !ok && attempts < poll.MaxAttempts {
                attempts++
                time.Sleep(time.Duration(poll.IntervalMS) * time.Millisecond)
                pollURL, err := n.render(poll.URL, lastBody)
                if err != nil { return nil, err }
                preq, _ := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
                for k, v := range hdrs { preq.Header.Set(k, v) }
                pr, err := client.Do(preq)
                if err != nil { return nil, err }
                func() { defer pr.Body.Close() }()
                pbytes, _ := io.ReadAll(pr.Body)
                var pbody any
                if json.Unmarshal(pbytes, &pbody) != nil { pbody = string(pbytes) }
                lastBody = pbody
                ok, err = done(lastBody)
                if err != nil { return nil, err }
            }
        }

        out = append(out, model.Item{
            "status":  resp.StatusCode,
            "body":    lastBody,
            "url":     urlStr,
            "node_id": node.ID,
        })
    }
    return out, nil
}

func init() { plugin.Register("http:request", func() plugin.NodeHandler { return &HttpRequest{} }) }

