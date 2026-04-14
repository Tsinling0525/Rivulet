package fs

import (
    "context"
    "encoding/json"
    "errors"
    "os"
    "path/filepath"
    "text/template"

    "github.com/Tsinling0525/rivulet/model"
    "github.com/Tsinling0525/rivulet/plugin"
)

// Write writes an item field to a file using a path template.
// Config:
// - path_template: string (Go template, required)
// - field: string (default: "body"); value may be string or map (JSON encoded)
// - mkdirs: bool (default: true)
type Write struct{ deps plugin.Deps }

func (n *Write) Init(ctx context.Context, deps plugin.Deps) error { n.deps = deps; return nil }

func (n *Write) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
    tplStr, _ := node.Config["path_template"].(string)
    if tplStr == "" { return nil, errors.New("path_template is required") }
    field, _ := node.Config["field"].(string)
    if field == "" { field = "body" }
    mkdirs := true
    if b, ok := node.Config["mkdirs"].(bool); ok { mkdirs = b }

    tpl, err := template.New("path").Parse(tplStr)
    if err != nil { return nil, err }

    out := make(model.Items, 0, len(in))
    for _, item := range in {
        if item == nil { item = model.Item{} }
        // render path
        var pathBuf bytesBuffer
        if err := tpl.Execute(&pathBuf, item); err != nil { return nil, err }
        path := pathBuf.String()
        if mkdirs { _ = os.MkdirAll(filepath.Dir(path), 0o755) }
        // extract content
        var data []byte
        switch v := item[field].(type) {
        case string:
            data = []byte(v)
        case []byte:
            data = v
        case map[string]any:
            b, err := json.Marshal(v)
            if err != nil { return nil, err }
            data = b
        default:
            b, err := json.Marshal(v)
            if err != nil { return nil, err }
            data = b
        }
        if err := os.WriteFile(path, data, 0o644); err != nil { return nil, err }
        // enrich
        o := model.Item{}
        for k, v := range item { o[k] = v }
        o["written_path"] = path
        out = append(out, o)
    }
    return out, nil
}

// small buffer for templates
type bytesBuffer struct{ b []byte }
func (w *bytesBuffer) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *bytesBuffer) String() string              { return string(w.b) }

func init() { plugin.Register("fs:write", func() plugin.NodeHandler { return &Write{} }) }

