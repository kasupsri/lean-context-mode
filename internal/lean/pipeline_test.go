package lean

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func runGit(t *testing.T, cwd string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineContextPack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs in linux path assumptions")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module sample\n")
	writeFile(t, filepath.Join(root, "src", "auth.go"), `package src

func Login(user string) string {
	return user
}

func ValidateToken(token string) bool {
	return token != ""
}
`)
	writeFile(t, filepath.Join(root, "src", "auth_test.go"), `package src

func TestLogin() { _ = Login("a") }
`)
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"sample"}`)

	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "ci@example.com")
	runGit(t, root, "config", "user.name", "ci")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")

	writeFile(t, filepath.Join(root, "src", "auth.go"), `package src

func Login(user string) string {
	return "ok:" + user
}

func ValidateToken(token string) bool {
	return token != ""
}
`)

	svc, err := NewService(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer svc.Stop()

	bundle := svc.ContextPack(ctx, ContextPackInput{
		Query:       "fix changed auth login behavior",
		FileHints:   []string{"src/auth.go"},
		Language:    "go",
		TokenBudget: 900,
	})
	if len(bundle.Symbols) == 0 {
		t.Fatalf("expected symbols in bundle")
	}
	if len(bundle.Snippets) == 0 {
		t.Fatalf("expected snippets in bundle")
	}
	if bundle.EstimatedTokens > bundle.TokenBudget+120 { // JSON overhead variance
		t.Fatalf("bundle exceeds budget unexpectedly: got=%d budget=%d", bundle.EstimatedTokens, bundle.TokenBudget)
	}
	focus := svc.ChangesFocus(ctx, ChangesFocusInput{})
	if len(focus.Files) == 0 {
		t.Fatalf("expected change tracking for modified file")
	}
}
