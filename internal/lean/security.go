package lean

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var controlChars = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F]`)

func SanitizeUserText(input string, maxLen int) string {
	clean := strings.TrimSpace(input)
	clean = controlChars.ReplaceAllString(clean, "")
	clean = strings.ReplaceAll(clean, "</tool>", "")
	clean = strings.ReplaceAll(clean, "<tool>", "")
	clean = strings.ReplaceAll(clean, "```", "` ` `")
	if len(clean) > maxLen {
		clean = clean[:maxLen]
	}
	return clean
}

func NormalizeWorkspacePath(root, candidate string) (string, string, error) {
	if strings.TrimSpace(candidate) == "" {
		return "", "", errors.New("path is empty")
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve root: %w", err)
	}
	candidate = filepath.FromSlash(strings.TrimSpace(candidate))
	var abs string
	if filepath.IsAbs(candidate) {
		abs = filepath.Clean(candidate)
	} else {
		abs = filepath.Clean(filepath.Join(cleanRoot, candidate))
	}
	if !strings.HasPrefix(abs, cleanRoot) {
		return "", "", fmt.Errorf("path escapes workspace root")
	}
	rel, err := filepath.Rel(cleanRoot, abs)
	if err != nil {
		return "", "", fmt.Errorf("relative path: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return "", "", fmt.Errorf("path escapes workspace root")
	}
	return abs, rel, nil
}

func ValidateTokenBudget(v int) int {
	if v == 0 {
		return DefaultTokenBudget
	}
	return clampInt(v, MinTokenBudget, MaxTokenBudget)
}
