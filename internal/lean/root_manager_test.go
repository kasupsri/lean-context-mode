package lean

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRootManagerSwitching(t *testing.T) {
	base := t.TempDir()
	repoA := filepath.Join(base, "repo-a")
	repoB := filepath.Join(base, "repo-b")
	repoBad := filepath.Join(t.TempDir(), "outside")
	mustMkRepo(t, repoA)
	mustMkRepo(t, repoB)
	mustMkRepo(t, repoBad)

	t.Setenv("LCM_ALLOWED_ROOTS", repoA+";"+repoB)
	rm, err := NewRootManager(context.Background(), repoA)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Stop()

	if got := rm.CurrentRoot(); filepath.Clean(got) != filepath.Clean(repoA) {
		t.Fatalf("unexpected active root: %s", got)
	}

	newRoot, err := rm.SetActiveRoot(repoB)
	if err != nil {
		t.Fatalf("set active root failed: %v", err)
	}
	if filepath.Clean(newRoot) != filepath.Clean(repoB) {
		t.Fatalf("unexpected new root: %s", newRoot)
	}

	if _, err := rm.SetActiveRoot(repoBad); err == nil {
		t.Fatalf("expected disallowed root to fail")
	}
}

func mustMkRepo(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
