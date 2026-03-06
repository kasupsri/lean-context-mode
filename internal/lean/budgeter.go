package lean

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type Budgeter struct{}

func NewBudgeter() *Budgeter { return &Budgeter{} }

func (b *Budgeter) Allocate(in BudgetInput) BudgetAllocation {
	query := strings.ToLower(SanitizeUserText(in.Query, 2000))
	budget := ValidateTokenBudget(in.TokenBudget)
	metadata := max(64, int(float64(budget)*0.08))
	effective := max(80, budget-metadata)

	weights := map[string]float64{
		"sig":    0.35,
		"snip":   0.30,
		"dep":    0.10,
		"diff":   0.20,
		"config": 0.05,
	}

	if hasAny(query, "fix", "regression", "changed", "diff", "patch", "broken", "failing") {
		weights["diff"] += 0.12
		weights["snip"] -= 0.08
		weights["dep"] -= 0.02
		weights["config"] -= 0.02
	}
	if hasAny(query, "definition", "signature", "interface", "type", "class", "function") {
		weights["sig"] += 0.12
		weights["snip"] -= 0.05
		weights["config"] -= 0.04
		weights["dep"] -= 0.03
	}
	if hasAny(query, "why", "flow", "dependency", "call", "stack", "trace") {
		weights["dep"] += 0.09
		weights["snip"] -= 0.05
		weights["config"] -= 0.04
	}
	if hasAny(strings.ToLower(in.Language), "terraform", "kubernetes", "yaml", "docker", "devops") {
		weights["config"] += 0.06
		weights["sig"] -= 0.03
		weights["snip"] -= 0.03
	}

	for k, v := range weights {
		if v < 0.03 {
			weights[k] = 0.03
		}
	}
	totalW := 0.0
	for _, v := range weights {
		totalW += v
	}
	for k, v := range weights {
		weights[k] = v / totalW
	}

	sig := int(float64(effective) * weights["sig"])
	snip := int(float64(effective) * weights["snip"])
	dep := int(float64(effective) * weights["dep"])
	diff := int(float64(effective) * weights["diff"])
	cfg := int(float64(effective) * weights["config"])

	remainder := effective - (sig + snip + dep + diff + cfg)
	for _, key := range []string{"sig", "diff", "snip", "dep", "config"} {
		if remainder <= 0 {
			break
		}
		switch key {
		case "sig":
			sig++
		case "diff":
			diff++
		case "snip":
			snip++
		case "dep":
			dep++
		case "config":
			cfg++
		}
		remainder--
	}

	summaryTokens := max(50, int(float64(budget)*0.10))
	seedRaw := fmt.Sprintf("%s|%s|%v|%d", query, strings.ToLower(in.Language), in.FileHints, budget)
	h := sha256.Sum256([]byte(seedRaw))

	return BudgetAllocation{
		TotalTokens:       budget,
		MetadataTokens:    metadata,
		SignaturesTokens:  sig,
		SnippetsTokens:    snip,
		DependencyTokens:  dep,
		DiffTokens:        diff,
		ConfigTokens:      cfg,
		SummaryTokens:     summaryTokens,
		EffectiveTokens:   effective,
		DeterministicSeed: hex.EncodeToString(h[:8]),
	}
}

func hasAny(s string, terms ...string) bool {
	for _, t := range terms {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
