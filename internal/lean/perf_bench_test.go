package lean

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkContextPack(b *testing.B) {
	root := benchmarkRepo(b)
	ctx := context.Background()
	svc, err := NewService(root)
	if err != nil {
		b.Fatal(err)
	}
	if err := svc.Start(ctx); err != nil {
		b.Fatal(err)
	}
	defer svc.Stop()

	in := ContextPackInput{
		Query:       "find api handlers and auth middleware",
		FileHints:   []string{"src"},
		Language:    "go",
		TokenBudget: 1400,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.ContextPack(ctx, in)
	}
}

func BenchmarkCodeSymbols(b *testing.B) {
	root := benchmarkRepo(b)
	ctx := context.Background()
	svc, err := NewService(root)
	if err != nil {
		b.Fatal(err)
	}
	if err := svc.Start(ctx); err != nil {
		b.Fatal(err)
	}
	defer svc.Stop()

	in := CodeSymbolsInput{Query: "handler", MaxSymbols: 100}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.CodeSymbols(ctx, in)
	}
}

func benchmarkRepo(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	mustWrite(tb, filepath.Join(root, "go.mod"), "module benchrepo\n")

	for i := 0; i < 120; i++ {
		content := "package app\n\n"
		content += fmt.Sprintf("func Handler%d(input string) string {\n", i)
		content += "\tif input == \"\" { return \"x\" }\n"
		content += fmt.Sprintf("\treturn Service%d(input)\n", i%10)
		content += "}\n\n"
		content += fmt.Sprintf("func Service%d(v string) string { return v }\n", i)
		mustWrite(tb, filepath.Join(root, "src", fmt.Sprintf("file_%03d.go", i)), content)
	}
	mustWrite(tb, filepath.Join(root, "package.json"), `{"name":"benchrepo","version":"1.0.0"}`)
	mustWrite(tb, filepath.Join(root, "config", "appsettings.json"), `{"FeatureFlags":{"A":true,"B":false}}`)
	return root
}

func mustWrite(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatal(err)
	}
}
