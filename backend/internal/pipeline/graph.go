package pipeline

import "fmt"

// GraphNode 流水线可视化中的一个节点。
type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // start | step | parallel | sub | end
	When  string `json:"when,omitempty"`
}

// GraphEdge 有向边（from → to）。
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Graph 流水线步骤的可视化图（按执行依赖连线，供前端布局渲染）。
type Graph struct {
	Image string      `json:"image,omitempty"`
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Graph 把配置转换为可渲染的 DAG：顶层步骤顺序连接；parallel 组以子节点展开。
func (c *Config) Graph() Graph {
	g := Graph{Image: c.Image, Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	g.Nodes = append(g.Nodes, GraphNode{ID: "start", Label: "start", Kind: "start"})
	prev := "start"
	for i, s := range c.Steps {
		id := fmt.Sprintf("step-%d", i)
		kind := "step"
		if len(s.Parallel) > 0 {
			kind = "parallel"
		}
		g.Nodes = append(g.Nodes, GraphNode{ID: id, Label: s.Name, Kind: kind, When: s.When})
		g.Edges = append(g.Edges, GraphEdge{From: prev, To: id})
		for j, sub := range s.Parallel {
			sid := fmt.Sprintf("step-%d-%d", i, j)
			g.Nodes = append(g.Nodes, GraphNode{ID: sid, Label: sub.Name, Kind: "sub", When: sub.When})
			g.Edges = append(g.Edges, GraphEdge{From: id, To: sid})
		}
		prev = id
	}
	g.Nodes = append(g.Nodes, GraphNode{ID: "end", Label: "end", Kind: "end"})
	g.Edges = append(g.Edges, GraphEdge{From: prev, To: "end"})
	return g
}
