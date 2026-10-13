package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnabmitra/pr-triage/internal/agent"
	"github.com/arnabmitra/pr-triage/internal/diff"
	"github.com/arnabmitra/pr-triage/internal/skills"
	"github.com/arnabmitra/pr-triage/internal/store"
	"github.com/arnabmitra/pr-triage/internal/ui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init-skills" {
		initSkills()
		return
	}

	mode := flag.String("mode", string(diff.ModeBoth), "what to review: branch | uncommitted | both")
	base := flag.String("base", "", "base ref (default: first of origin/main, origin/master, main, master)")
	model := flag.String("model", "", "cursor-agent model, e.g. claude-opus-4-8 (default: auto)")
	bin := flag.String("agent-bin", "cursor-agent", "cursor-agent binary")
	noAgent := flag.Bool("no-agent", false, "heuristic ranking only; disables quiz and fix")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: pr-triage [flags]\n       pr-triage init-skills\n\nflags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	repo, err := diff.RepoRoot(".")
	if err != nil {
		fatal(err)
	}
	gitDir, err := diff.GitCommonDir(repo)
	if err != nil {
		fatal(err)
	}
	st, err := store.Open(filepath.Join(gitDir, "pr-triage", "state.json"))
	if err != nil {
		fatal(err)
	}

	m := ui.New(ui.Options{
		Agent:   &agent.Client{Bin: *bin, Model: *model, Repo: repo},
		Base:    *base,
		Mode:    diff.Mode(*mode),
		NoAgent: *noAgent,
		Repo:    repo,
		Store:   st,
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "pr-triage:", err)
	os.Exit(1)
}

func initSkills() {
	dir, err := skills.UserDir()
	if err != nil {
		fatal(err)
	}
	written, err := skills.WriteDefaults(dir)
	if err != nil {
		fatal(err)
	}
	if len(written) == 0 {
		fmt.Println("skills already exist in", dir)
		return
	}
	for _, p := range written {
		fmt.Println("wrote", p)
	}
	fmt.Println("\nEdit these to tune ranking, quizzing, grading, and fixing.")
	fmt.Println("Per-repo overrides go in <repo>/.pr-triage/skills/<name>.md")
}
