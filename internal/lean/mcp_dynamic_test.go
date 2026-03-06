package lean

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
	want := []string{"cache.clean", "changes.focus", "code.snippet", "code.symbols", "context.pack", "repo.map", "workspace.root.get", "workspace.root.set"}
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
	var setOut WorkspaceRootOutput
	if err := json.Unmarshal([]byte(setText), &setOut); err != nil {
		t.Fatalf("workspace.root.set response is not valid json: %v, body=%s", err, setText)
	}
	if filepath.Clean(setOut.ActiveRoot) != filepath.Clean(repoB) {
		t.Fatalf("workspace.root.set response root mismatch got=%q want=%q", setOut.ActiveRoot, repoB)
	}

	cleanRes, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cache.clean",
		Arguments: map[string]any{"workspace_root": repoB, "mode": "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleanRes.IsError {
		t.Fatalf("cache.clean returned error: %+v", cleanRes)
	}
	cleanText := cleanRes.Content[0].(*mcp.TextContent).Text
	var cleanOut CacheCleanOutput
	if err := json.Unmarshal([]byte(cleanText), &cleanOut); err != nil {
		t.Fatalf("cache.clean response is not valid json: %v, body=%s", err, cleanText)
	}
	if filepath.Clean(cleanOut.Root) != filepath.Clean(repoB) {
		t.Fatalf("cache.clean root mismatch got=%q want=%q", cleanOut.Root, repoB)
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
