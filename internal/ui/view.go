package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/arnabmitra/pr-triage/internal/triage"
)

var (
	addStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("3")).Padding(1, 2)
	busyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("4"))
	hunkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	labelStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	listStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderRight(true).BorderForeground(lipgloss.Color("8"))
	removeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6"))
	titleStyle    = lipgloss.NewStyle().Bold(true)
)

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	var body string
	switch m.screen {
	case screenBrowse:
		body = m.renderBrowse()
	case screenConfirmFix:
		body = m.renderConfirmFix()
	case screenOutput:
		body = m.output.View()
	case screenQuiz:
		body = m.renderQuiz()
	}
	h := m.bodyHeight()
	body = lipgloss.NewStyle().Height(h).MaxHeight(h).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), body, m.renderFooter())
}

func (m Model) renderBrowse() string {
	if m.zoom {
		return m.viewport.View()
	}
	h := m.bodyHeight()
	w := m.listWidth()
	vis := m.visible()
	sel := clamp(m.selected, 0, len(vis)-1)
	offset := clamp(sel-h/2, 0, max(0, len(vis)-h))

	rows := make([]string, 0, h)
	for i := offset; i < len(vis) && i < offset+h; i++ {
		rows = append(rows, m.renderRow(m.items[vis[i]], i == sel, w))
	}
	list := listStyle.Width(w).Height(h).Render(strings.Join(rows, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", m.viewport.View())
}

func (m Model) renderConfirmFix() string {
	it := m.current()
	if it == nil {
		return ""
	}
	w := min(80, m.width-6)
	content := strings.Join([]string{
		titleStyle.Render("Let cursor-agent fix this?"),
		"",
		wrap(it.finding.Title, w),
		dimStyle.Render(wrap(it.finding.Why, w)),
		"",
		"It runs with --force in " + m.opts.Repo + " and edits files directly.",
		"",
		addStyle.Render("y") + " fix   " + removeStyle.Render("n") + " cancel",
	}, "\n")
	return lipgloss.Place(m.width, m.bodyHeight(), lipgloss.Center, lipgloss.Center, boxStyle.Width(w).Render(content))
}

func (m Model) renderDetail(it item, w int) string {
	f := it.finding
	hs := m.opts.Store.Hunk(it.hunk.Key)
	var b strings.Builder

	b.WriteString(titleStyle.Render(wrap(f.Title, w)) + "\n")
	b.WriteString(fmt.Sprintf("%s  %s  %s  %s  %s\n",
		scoreStyle(f.Score()).Render(fmt.Sprintf("score %.1f", f.Score())),
		scoreStyle(f.Risk).Render(fmt.Sprintf("risk %.0f", f.Risk)),
		scoreStyle(f.Immovable).Render(fmt.Sprintf("immovable %.0f", f.Immovable)),
		scoreStyle(f.Regret).Render(fmt.Sprintf("regret %.0f", f.Regret)),
		dimStyle.Render("("+f.Source+")"),
	))
	b.WriteString(dimStyle.Render(fmt.Sprintf("%s:%d  +%d −%d", it.hunk.File, it.hunk.StartLine, it.hunk.Added, it.hunk.Removed)) + "\n")
	if hs.Reviewed || hs.Quizzed {
		var status []string
		if hs.Reviewed {
			status = append(status, "reviewed")
		}
		if hs.Quizzed {
			status = append(status, fmt.Sprintf("quiz %d/100", hs.QuizScore))
		}
		b.WriteString(addStyle.Render(strings.Join(status, " · ")) + "\n")
	}
	if f.Why != "" {
		b.WriteString("\n" + labelStyle.Render("Why") + "\n" + wrap(f.Why, w) + "\n")
	}
	if f.Check != "" {
		b.WriteString("\n" + labelStyle.Render("Check") + "\n" + wrap(f.Check, w) + "\n")
	}
	b.WriteString("\n" + hunkStyle.Render(ansi.Truncate(it.hunk.Header, w, "…")) + "\n")
	for _, l := range it.hunk.Lines {
		l = ansi.Truncate(strings.ReplaceAll(l, "\t", "    "), w, "…")
		switch {
		case strings.HasPrefix(l, "+"):
			l = addStyle.Render(l)
		case strings.HasPrefix(l, "-"):
			l = removeStyle.Render(l)
		default:
			l = dimStyle.Render(l)
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

func (m Model) renderFooter() string {
	var s string
	switch {
	case m.busy != "":
		s = m.spinner.View() + " " + busyStyle.Render(m.busy)
	case m.errMsg != "":
		s = errStyle.Render(m.errMsg)
	case m.notice != "":
		s = addStyle.Render(m.notice)
	default:
		s = dimStyle.Render(m.help())
	}
	return ansi.Truncate(s, m.width, "…")
}

func (m Model) renderHeader() string {
	reviewed := 0
	for _, it := range m.items {
		if m.opts.Store.Hunk(it.hunk.Key).Reviewed {
			reviewed++
		}
	}
	parts := []string{" pr-triage", m.desc, fmt.Sprintf("reviewed %d/%d", reviewed, len(m.items)), "sort:" + sortNames[m.sortBy]}
	if m.hideReviewed {
		parts = append(parts, "hiding reviewed")
	}
	return headerStyle.Width(m.width).Render(ansi.Truncate(strings.Join(parts, "  │  "), m.width, "…"))
}

func (m Model) renderQuiz() string {
	it := m.itemByKey(m.quiz.key)
	if it == nil || m.quiz.idx >= len(m.quiz.questions) {
		return ""
	}
	w := min(m.width-4, 110)
	q := m.quiz.questions[m.quiz.idx]
	var b strings.Builder
	b.WriteString(dimStyle.Render(fmt.Sprintf("Quiz %d/%d · %s", m.quiz.idx+1, len(m.quiz.questions), it.finding.Title)) + "\n\n")
	b.WriteString(titleStyle.Render(wrap(q.Q, w)) + "\n\n")
	b.WriteString(m.textarea.View() + "\n")
	if g := m.quiz.grade; g != nil {
		b.WriteString("\n" + scoreStyle(10-float64(g.Score)/10).Render(fmt.Sprintf("%s · %d/100", g.Verdict, g.Score)) + "\n")
		b.WriteString(wrap(g.Feedback, w) + "\n\n")
		b.WriteString(labelStyle.Render("Model answer") + "\n" + wrap(g.Answer, w) + "\n")
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m Model) renderRow(it item, selected bool, w int) string {
	hs := m.opts.Store.Hunk(it.hunk.Key)
	mark := "  "
	if hs.Reviewed {
		mark = "✓ "
	}
	quiz := " "
	if hs.Quizzed {
		quiz = "?"
		if hs.QuizScore >= passingQuizScore {
			quiz = "★"
		}
	}
	src := " "
	if it.finding.Source == triage.SourceHeuristic {
		src = "~"
	}
	title := it.finding.Title
	if title == "" {
		title = it.hunk.File
	}
	score := fmt.Sprintf("%4.1f%s", it.finding.Score(), src)
	plain := ansi.Truncate(fmt.Sprintf("%s%s %s %s", mark, quiz, score, title), w, "…")
	switch {
	case selected:
		return selectedStyle.Width(w).Render(plain)
	case hs.Reviewed:
		return dimStyle.Render(plain)
	}
	prefix := mark + quiz + " "
	rest := strings.TrimPrefix(plain, prefix+score)
	return prefix + scoreStyle(it.finding.Score()).Render(score) + rest
}

func (m Model) help() string {
	switch m.screen {
	case screenQuiz:
		if m.quiz.grade != nil {
			return "enter next question · esc back (progress kept)"
		}
		return "ctrl+s submit answer · esc back (progress kept)"
	case screenOutput:
		return "j/k scroll · enter/esc back and reload diff"
	case screenConfirmFix:
		return "y fix · n cancel"
	}
	if m.zoom {
		return "j/k next/prev hunk · y copy finding · Y copy + diff · J/K ctrl+d/u scroll · enter/esc back to list"
	}
	return "j/k move · space reviewed · y/Y copy · enter zoom · t quiz me · f fix · tab sort · H hide reviewed · J/K ctrl+d/u scroll · r re-rank · R reload · q quit   (~ = heuristic)"
}

func scoreStyle(v float64) lipgloss.Style {
	switch {
	case v >= 7:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	case v >= 4:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	}
}

func wrap(s string, w int) string {
	if w <= 0 {
		return s
	}
	return lipgloss.NewStyle().Width(w).Render(s)
}
