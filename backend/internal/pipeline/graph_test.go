package pipeline

import "testing"

func TestConfigGraph(t *testing.T) {
	cfg, err := Parse([]byte(`
image: alpine:3.19
steps:
  - name: build
    run: echo build
  - name: checks
    parallel:
      - name: lint
        run: echo lint
      - name: test
        when: event == "push"
        run: echo test
`))
	if err != nil {
		t.Fatal(err)
	}
	g := cfg.Graph()
	// start + build + checks(parallel) + lint + test + end
	if len(g.Nodes) != 6 {
		t.Fatalf("nodes = %d: %+v", len(g.Nodes), g.Nodes)
	}
	// start->build, build->checks, checks->lint, checks->test, checks->end
	if len(g.Edges) != 5 {
		t.Fatalf("edges = %d: %+v", len(g.Edges), g.Edges)
	}
	if g.Image != "alpine:3.19" {
		t.Fatalf("image = %q", g.Image)
	}
	kinds := map[string]int{}
	for _, n := range g.Nodes {
		kinds[n.Kind]++
	}
	if kinds["parallel"] != 1 || kinds["sub"] != 2 {
		t.Fatalf("kinds = %+v", kinds)
	}
}
