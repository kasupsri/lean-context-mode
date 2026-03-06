package lean

import "testing"

func TestBudgeterDeterministic(t *testing.T) {
	b := NewBudgeter()
	in := BudgetInput{Query: "find auth service interface", FileHints: []string{"src/auth"}, Language: "go", TokenBudget: 1800}
	a := b.Allocate(in)
	b2 := b.Allocate(in)
	if a != b2 {
		t.Fatalf("allocation not deterministic: %#v vs %#v", a, b2)
	}
	if a.TotalTokens != 1800 {
		t.Fatalf("unexpected total tokens: %d", a.TotalTokens)
	}
	if a.SignaturesTokens+a.SnippetsTokens+a.DependencyTokens+a.DiffTokens+a.ConfigTokens != a.EffectiveTokens {
		t.Fatalf("allocation sum mismatch")
	}
}

func TestBudgeterDiffPriority(t *testing.T) {
	b := NewBudgeter()
	base := b.Allocate(BudgetInput{Query: "explain architecture", TokenBudget: 1200})
	diff := b.Allocate(BudgetInput{Query: "why failing tests after changed diff", TokenBudget: 1200})
	if diff.DiffTokens <= base.DiffTokens {
		t.Fatalf("expected diff tokens to increase: base=%d diff=%d", base.DiffTokens, diff.DiffTokens)
	}
}
