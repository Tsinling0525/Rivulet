package logic

import (
	"context"
	"text/template"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

// If node routes items to "true" or "false" port based on an expression template returning "true"/"false" string
// Config: expr (string, Go template that should render to "true" or "false")
type If struct{ deps plugin.Deps }

func (n *If) Init(ctx context.Context, deps plugin.Deps) error { n.deps = deps; return nil }

type ported interface {
	ProcessPorted(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (map[model.Port]model.Items, error)
}

func (n *If) ProcessPorted(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (map[model.Port]model.Items, error) {
	expr, _ := node.Config["expr"].(string)
	tpl, err := template.New("expr").Parse(expr)
	if err != nil {
		return nil, err
	}

	trueItems := model.Items{}
	falseItems := model.Items{}
	for _, it := range in {
		if it == nil {
			it = model.Item{}
		}
		var buf bytesBuffer
		if err := tpl.Execute(&buf, it); err != nil {
			return nil, err
		}
		if buf.String() == "true" {
			trueItems = append(trueItems, it)
		} else {
			falseItems = append(falseItems, it)
		}
	}
	return map[model.Port]model.Items{
		model.Port("true"):  trueItems,
		model.Port("false"): falseItems,
		model.PortMain:      append(model.Items{}, in...),
	}, nil
}

// allow registration without explicit engine import
func (n *If) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	return in, nil
}

func init() { plugin.Register("logic:if", func() plugin.NodeHandler { return &If{} }) }

// tiny buffer for templates
type bytesBuffer struct{ b []byte }

func (w *bytesBuffer) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *bytesBuffer) String() string              { return string(w.b) }
