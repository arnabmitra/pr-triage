package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/muesli/termenv"
)

// copyToClipboard prefers the system clipboard locally and OSC 52 over SSH, where the
// system clipboard would belong to the remote machine.
func copyToClipboard(s string) {
	if os.Getenv("SSH_TTY") == "" {
		if err := clipboard.WriteAll(s); err == nil {
			return
		}
	}
	termenv.Copy(s)
}

func findingMarkdown(it item, withDiff bool) string {
	f := it.finding
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** (score %.1f: risk %.0f, immovable %.0f, regret %.0f)\n", f.Title, f.Score(), f.Risk, f.Immovable, f.Regret)
	fmt.Fprintf(&b, "`%s:%d` (+%d −%d)\n", it.hunk.File, it.hunk.StartLine, it.hunk.Added, it.hunk.Removed)
	if f.Why != "" {
		fmt.Fprintf(&b, "\nWhy: %s\n", f.Why)
	}
	if f.Check != "" {
		fmt.Fprintf(&b, "Check: %s\n", f.Check)
	}
	if withDiff {
		fmt.Fprintf(&b, "\n```diff\n%s\n%s\n```\n", it.hunk.Header, strings.Join(it.hunk.Lines, "\n"))
	}
	return b.String()
}
