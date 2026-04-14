package files

import (
    "context"
    "encoding/base64"
    "path/filepath"

    "github.com/Tsinling0525/rivulet/model"
    "github.com/Tsinling0525/rivulet/plugin"
)

// Load reads a file from the FileStore using item[file_id] and attaches
// metadata and bytes to the item.
// Config:
// - file_id_field: string (default: "file_id")
// - out_prefix: string (default: "file_") => fields: <prefix>name, <prefix>bytes, <prefix>media_type, <prefix>ext, <prefix>base
type Load struct{ deps plugin.Deps }

func (n *Load) Init(ctx context.Context, deps plugin.Deps) error { n.deps = deps; return nil }

func (n *Load) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
    if n.deps.Files == nil { return nil, ErrNoFiles }
    idField := "file_id"
    if v, ok := node.Config["file_id_field"].(string); ok && v != "" { idField = v }
    prefix := "file_"
    if v, ok := node.Config["out_prefix"].(string); ok && v != "" { prefix = v }

    out := make(model.Items, 0, len(in))
    for _, item := range in {
        if item == nil { item = model.Item{} }
        raw := item[idField]
        fid, _ := raw.(string)
        name, mt, data, err := n.deps.Files.Get(ctx, string(wf.ID), fid)
        if err != nil { return nil, err }
        base := filepath.Base(name)
        ext := filepath.Ext(name)

        // Clone item and attach
        o := model.Item{}
        for k, v := range item { o[k] = v }
        o[prefix+"name"] = name
        o[prefix+"base"] = base
        o[prefix+"ext"] = ext
        o[prefix+"media_type"] = mt
        o[prefix+"bytes"] = data
        o[prefix+"b64"] = base64.StdEncoding.EncodeToString(data)
        out = append(out, o)
    }
    return out, nil
}

var ErrNoFiles = pluginError("files store not configured")

type pluginError string

func (e pluginError) Error() string { return string(e) }

func init() { plugin.Register("files:load", func() plugin.NodeHandler { return &Load{} }) }
