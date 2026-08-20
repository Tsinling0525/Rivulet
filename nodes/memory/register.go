package memory

import "github.com/Tsinling0525/rivulet/plugin"

func init() {
	plugin.Register("memory:write", func() plugin.NodeHandler { return &Write{} })
	plugin.Register("memory:update", func() plugin.NodeHandler { return &Update{} })
	plugin.Register("memory:query", func() plugin.NodeHandler { return &Query{} })
}
