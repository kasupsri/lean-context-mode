package lean

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func RunMCPServerDynamic(ctx context.Context, rm *RootManager, version string) error {
	server := BuildMCPServerDynamic(rm, version)
	return server.Run(ctx, &mcp.StdioTransport{})
}

func BuildMCPServerDynamic(rm *RootManager, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "lean-context-mode", Version: version}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "context.pack",
		Description: "Run budgeter->retriever->summarizer pipeline and return a minimal context bundle",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ContextPackInput) (*mcp.CallToolResult, ContextBundle, error) {
		svc, _, err := rm.ServiceFor(in.WorkspaceRoot)
		if err != nil {
			return toolErrResult(err), ContextBundle{}, nil
		}
		bundle := svc.ContextPack(ctx, in)
		text := compactJSON(bundle)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, bundle, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "code.symbols",
		Description: "Return symbol signatures and source locations with language-agnostic heuristics",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CodeSymbolsInput) (*mcp.CallToolResult, CodeSymbolsOutput, error) {
		svc, _, err := rm.ServiceFor(in.WorkspaceRoot)
		if err != nil {
			return toolErrResult(err), CodeSymbolsOutput{}, nil
		}
		out := svc.CodeSymbols(ctx, in)
		text := compactJSON(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "code.snippet",
		Description: "Return a validated snippet for a file + line range inside workspace root",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CodeSnippetInput) (*mcp.CallToolResult, CodeSnippetOutput, error) {
		svc, _, err := rm.ServiceFor(in.WorkspaceRoot)
		if err != nil {
			return toolErrResult(err), CodeSnippetOutput{}, nil
		}
		out, err := svc.CodeSnippet(ctx, in)
		if err != nil {
			return toolErrResult(err), CodeSnippetOutput{}, nil
		}
		text := compactJSON(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "repo.map",
		Description: "Return a compact repository overview (languages, symbols, relationships, heuristics)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RepoMapInput) (*mcp.CallToolResult, RepoMap, error) {
		svc, _, err := rm.ServiceFor(in.WorkspaceRoot)
		if err != nil {
			return toolErrResult(err), RepoMap{}, nil
		}
		out := svc.RepoMap(ctx, in)
		text := compactJSON(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "changes.focus",
		Description: "Return git diff hunks and affected symbols with diff-first prioritization",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ChangesFocusInput) (*mcp.CallToolResult, ChangesFocus, error) {
		svc, _, err := rm.ServiceFor(in.WorkspaceRoot)
		if err != nil {
			return toolErrResult(err), ChangesFocus{}, nil
		}
		out := svc.ChangesFocus(ctx, in)
		text := compactJSON(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace.root.get",
		Description: "Get active workspace root and allowed root boundaries",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceRootGetInput) (*mcp.CallToolResult, WorkspaceRootOutput, error) {
		out := WorkspaceRootOutput{ActiveRoot: rm.CurrentRoot(), AllowedRoots: rm.AllowedRoots()}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: compactJSON(out)}}}, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace.root.set",
		Description: "Set active workspace root (must be inside allowed roots)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceRootSetInput) (*mcp.CallToolResult, WorkspaceRootOutput, error) {
		root, err := rm.SetActiveRoot(in.WorkspaceRoot)
		if err != nil {
			return toolErrResult(err), WorkspaceRootOutput{}, nil
		}
		out := WorkspaceRootOutput{ActiveRoot: root, AllowedRoots: rm.AllowedRoots()}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: compactJSON(out)}}}, out, nil
	})

	return server
}

func toolErrResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "error: " + err.Error()}}, IsError: true}
}
