package triage

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/arnabmitra/pr-triage/internal/diff"
)

const (
	SourceAgent     = "agent"
	SourceHeuristic = "heuristic"
)

var exportedSig = regexp.MustCompile(`(?m)^[-+]\s*(func (\([^)]*\) )?[A-Z]\w*\(|type [A-Z]\w* |export (async )?(function|const|class|interface|type) )`)

type Finding struct {
	Check     string  `json:"check"`
	ID        string  `json:"id"`
	Immovable float64 `json:"immovable"`
	Regret    float64 `json:"regret"`
	Risk      float64 `json:"risk"`
	Source    string  `json:"source"`
	Title     string  `json:"title"`
	Why       string  `json:"why"`
}

// Score weights the three axes into a single priority (0-10).
func (f Finding) Score() float64 {
	return 0.4*f.Risk + 0.35*f.Immovable + 0.25*f.Regret
}

// Heuristic gives a fast first-pass ranking before (or without) the agent.
func Heuristic(h diff.Hunk) Finding {
	f := Finding{ID: h.ID, Source: SourceHeuristic}
	var reasons []string
	bump := func(dst *float64, v float64, reason string) {
		*dst = math.Max(*dst, v)
		reasons = append(reasons, reason)
	}

	path := strings.ToLower(h.File)
	changed := strings.ToLower(h.ChangedText())

	switch {
	case containsAny(path, "migration", "/schema/", ".sql"):
		bump(&f.Immovable, 8, "schema/migration")
	case containsAny(path, ".proto", "openapi", "swagger", ".abi", ".idl", ".sol", "/idl/"):
		bump(&f.Immovable, 7, "interface definition")
	case containsAny(path, ".yaml", ".yml", ".env", "dockerfile", ".github/", "makefile", ".toml"):
		bump(&f.Risk, 5, "config/build")
	}
	if h.Deleted {
		bump(&f.Regret, 6, "file deleted")
	}
	if exportedSig.MatchString(h.ChangedText()) {
		bump(&f.Immovable, 6, "exported API changed")
	}
	if containsAny(changed, "drop ", "alter table", "delete from", "truncate ") {
		bump(&f.Regret, 9, "destructive SQL")
	}
	if containsAny(changed, "transfer", "withdraw", "amount", "balance", "mint", "burn", "redeem") {
		bump(&f.Regret, 7, "money movement")
	}
	if containsAny(changed, "auth", "permission", "secret", "private_key", "privatekey", "password") {
		bump(&f.Regret, 6, "auth/secrets")
	}
	if containsAny(changed, "go func", "mutex", "sync.", "chan ", ".lock()") {
		bump(&f.Risk, 6, "concurrency")
	}
	if h.Removed > 10 && h.Removed > 2*h.Added {
		bump(&f.Regret, 5, "mostly deletions")
	}
	f.Risk = math.Max(f.Risk, math.Min(4, float64(h.Added+h.Removed)/25))

	if containsAny(path, "_test.", ".test.", "/testdata/", ".md", "/docs/") {
		f.Risk, f.Immovable, f.Regret = f.Risk*0.3, f.Immovable*0.3, f.Regret*0.3
		reasons = append(reasons, "tests/docs (discounted)")
	}

	f.Title = fmt.Sprintf("%s:%d (+%d −%d)", h.File, h.StartLine, h.Added, h.Removed)
	f.Why = "no obvious risk signals"
	if len(reasons) > 0 {
		f.Why = "heuristic: " + strings.Join(reasons, ", ")
	}
	return f
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
