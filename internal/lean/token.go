package lean

import (
	"regexp"
	"strings"
)

var codeHeavyHint = regexp.MustCompile(`(?m)(^\s*(func|class|interface|type|import|package|def|fn|resource|module|kind):?|=>|\{\s*$|\breturn\b)`) //nolint:lll

func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	charsPerToken := 4.0
	if codeHeavyHint.MatchString(text) {
		charsPerToken = 3.2
	}
	return int(float64(len(text))/charsPerToken) + 1
}

func EstimateTokensLines(lines []string) int {
	if len(lines) == 0 {
		return 0
	}
	return EstimateTokens(strings.Join(lines, "\n"))
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
