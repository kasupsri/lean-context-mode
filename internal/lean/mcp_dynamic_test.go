package lean

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPDynamicToolSurfaceAndRootSet(t *testing.T) {
	base := t.TempDir()
	repoA := filepath.Join(base, "repo-a")
	repoB := filepath.Join(base, "repo-b")
	mustMkRepo(t, repoA)
	mustMkRepo(t, repoB)

	t.Setenv("LCM_ALLOWED_ROOTS", repoA+";"+repoB)
	ctx := context.Background()
	rm, err := NewRootManager(ctx, repoA)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Stop()

	server := BuildMCPServerDynamic(rm, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(list.Tools))
	for _, tool := range list.Tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)
	want := []string{"changes.focus", "code.snippet", "code.symbols", "context.pack", "repo.map", "workspace.root.get", "workspace.root.set"}
	if len(got) != len(want) {
		t.Fatalf("tool count mismatch got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tools mismatch got=%v want=%v", got, want)
		}
	}

	setRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "workspace.root.set", Arguments: map[string]any{"workspace_root": repoB}})
	if err != nil {
		t.Fatal(err)
	}
	if setRes.IsError {
		t.Fatalf("workspace.root.set returned error: %+v", setRes)
	}
	setText := setRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(setText, filepath.ToSlash(repoB)) && !strings.Contains(setText, repoB) {
		t.Fatalf("workspace.root.set response missing new root: %s", setText)
	}

	packRes, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "context.pack",
		Arguments: map[string]any{
			"query":          "find main function",
			"workspace_root": repoB,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if packRes.IsError {
		t.Fatalf("context.pack returned error: %+v", packRes)
	}
	if len(packRes.Content) == 0 {
		t.Fatalf("context.pack returned empty content")
	}

	if _, err := os.Stat(filepath.Join(repoB, ".lean-context-mode", "metrics.json")); err != nil {
		t.Fatalf("expected metrics file in switched root: %v", err)
	}
}
