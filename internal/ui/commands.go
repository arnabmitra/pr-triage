package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnabmitra/pr-triage/internal/agent"
	"github.com/arnabmitra/pr-triage/internal/diff"
	"github.com/arnabmitra/pr-triage/internal/skills"
	"github.com/arnabmitra/pr-triage/internal/triage"
)

type diffLoadedMsg struct {
	desc  string
	err   error
	hunks []diff.Hunk
}

type fixMsg struct {
	err    error
	output string
}

type gradeMsg struct {
	err   error
	grade agent.Grade
}

type quizMsg struct {
	err       error
	key       string
	questions []agent.Question
}

type rankedMsg struct {
	err         error
	fingerprint string
	// findings is keyed by hunk Key so results survive a diff reload mid-flight.
	findings map[string]triage.Finding
}

func cacheKey(hunkKey, fingerprint string) string {
	return hunkKey + ":" + fingerprint
}

func fingerprint(skill, model string) string {
	sum := sha256.Sum256([]byte(skill + "\x00" + model))
	return hex.EncodeToString(sum[:6])
}

func fixCmd(opts Options, it item) tea.Cmd {
	return func() tea.Msg {
		skill, _, err := skills.Load(opts.Repo, skills.Fix)
		if err != nil {
			return fixMsg{err: err}
		}
		out, err := opts.Agent.Fix(context.Background(), skill, it.hunk, it.finding)
		return fixMsg{err: err, output: out}
	}
}

func gradeCmd(opts Options, it item, q agent.Question, answer string) tea.Cmd {
	return func() tea.Msg {
		skill, _, err := skills.Load(opts.Repo, skills.Grade)
		if err != nil {
			return gradeMsg{err: err}
		}
		g, err := opts.Agent.Grade(context.Background(), skill, it.hunk, q, answer)
		return gradeMsg{err: err, grade: g}
	}
}

func loadDiffCmd(opts Options) tea.Cmd {
	return func() tea.Msg {
		hunks, desc, err := diff.Load(opts.Repo, opts.Mode, opts.Base)
		return diffLoadedMsg{desc: desc, err: err, hunks: hunks}
	}
}

func quizCmd(opts Options, it item) tea.Cmd {
	return func() tea.Msg {
		skill, _, err := skills.Load(opts.Repo, skills.Quiz)
		if err != nil {
			return quizMsg{err: err}
		}
		qs, err := opts.Agent.Quiz(context.Background(), skill, it.hunk, it.finding)
		return quizMsg{err: err, key: it.hunk.Key, questions: qs}
	}
}

func rankCmd(opts Options, hunks []diff.Hunk) tea.Cmd {
	return func() tea.Msg {
		skill, _, err := skills.Load(opts.Repo, skills.Rank)
		if err != nil {
			return rankedMsg{err: err}
		}
		byID, err := opts.Agent.Rank(context.Background(), skill, hunks)
		byKey := map[string]triage.Finding{}
		for _, h := range hunks {
			if f, ok := byID[h.ID]; ok {
				byKey[h.Key] = f
			}
		}
		return rankedMsg{err: err, fingerprint: fingerprint(skill, opts.Agent.Model), findings: byKey}
	}
}

func rankFingerprint(opts Options) string {
	skill, _, _ := skills.Load(opts.Repo, skills.Rank)
	return fingerprint(skill, opts.Agent.Model)
}
