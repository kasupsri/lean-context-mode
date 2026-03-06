package lean

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func RunMCPServer(ctx context.Context, svc *Service, version string) error {
	server := BuildMCPServer(svc, version)
	return server.Run(ctx, &mcp.StdioTransport{})
}

func BuildMCPServer(svc *Service, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "lean-context-mode", Version: version}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "context.pack",
		Description: "Run budgeter->retriever->summarizer pipeline and return a minimal context bundle",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ContextPackInput) (*mcp.CallToolResult, ContextBundle, error) {
		bundle := svc.ContextPack(ctx, in)
		text := compactJSON(bundle)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, bundle, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "code.symbols",
		Description: "Return symbol signatures and source locations with language-agnostic heuristics",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CodeSymbolsInput) (*mcp.CallToolResult, CodeSymbolsOutput, error) {
		out := svc.CodeSymbols(ctx, in)
		text := compactJSON(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "code.snippet",
		Description: "Return a validated snippet for a file + line range inside workspace root",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CodeSnippetInput) (*mcp.CallToolResult, CodeSnippetOutput, error) {
		out, err := svc.CodeSnippet(ctx, in)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "error: " + err.Error()}},
				IsError: true,
			}, CodeSnippetOutput{}, nil
		}
		text := compactJSON(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "repo.map",
		Description: "Return a compact repository overview (languages, symbols, relationships, heuristics)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RepoMapInput) (*mcp.CallToolResult, RepoMap, error) {
		out := svc.RepoMap(ctx, in)
		text := compactJSON(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "changes.focus",
		Description: "Return git diff hunks and affected symbols with diff-first prioritization",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ChangesFocusInput) (*mcp.CallToolResult, ChangesFocus, error) {
		out := svc.ChangesFocus(ctx, in)
		text := compactJSON(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
	})

	return server
}

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}
