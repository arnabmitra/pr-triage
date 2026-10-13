package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnabmitra/pr-triage/internal/agent"
	"github.com/arnabmitra/pr-triage/internal/diff"
	"github.com/arnabmitra/pr-triage/internal/store"
	"github.com/arnabmitra/pr-triage/internal/triage"
)

const passingQuizScore = 70

type screen int

const (
	screenBrowse screen = iota
	screenConfirmFix
	screenOutput
	screenQuiz
)

type sortKey int

const (
	sortScore sortKey = iota
	sortRisk
	sortImmovable
	sortRegret
	sortFile
)

var sortNames = []string{"score", "risk", "immovable", "regret", "file"}

type Options struct {
	Agent   *agent.Client
	Base    string
	Mode    diff.Mode
	NoAgent bool
	Repo    string
	Store   *store.Store
}

type item struct {
	finding triage.Finding
	hunk    diff.Hunk
}

type quizState struct {
	grade     *agent.Grade
	idx       int
	key       string
	questions []agent.Question
	scores    []int
}

type Model struct {
	busy         string
	desc         string
	errMsg       string
	height       int
	hideReviewed bool
	items        []item
	notice       string
	opts         Options
	output       viewport.Model
	quiz         quizState
	screen       screen
	selected     int
	sortBy       sortKey
	spinner      spinner.Model
	textarea     textarea.Model
	viewport     viewport.Model
	width        int
	// zoom shows the detail pane full-width so mouse selection doesn't pick up the list.
	zoom bool
}

func New(opts Options) Model {
	ta := textarea.New()
	ta.Placeholder = "Explain it like you're defending this in review…"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = busyStyle

	return Model{
		busy:     "loading diff…",
		opts:     opts,
		output:   viewport.New(0, 0),
		spinner:  sp,
		textarea: ta,
		viewport: viewport.New(0, 0),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, loadDiffCmd(m.opts))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case diffLoadedMsg:
		return m.onDiffLoaded(msg)
	case rankedMsg:
		return m.onRanked(msg)
	case quizMsg:
		return m.onQuiz(msg)
	case gradeMsg:
		m.busy = ""
		if msg.err != nil {
			m.errMsg = "grade: " + msg.err.Error()
			return m, nil
		}
		m.errMsg = ""
		m.quiz.grade = &msg.grade
		m.quiz.scores = append(m.quiz.scores, msg.grade.Score)
		return m, nil
	case fixMsg:
		m.busy = ""
		content := msg.output
		if msg.err != nil {
			content = "fix failed:\n\n" + msg.err.Error()
		}
		m.output.SetContent(wrap(content, m.output.Width))
		m.output.GotoTop()
		m.screen = screenOutput
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.screen {
		case screenBrowse:
			return m.updateBrowse(msg)
		case screenConfirmFix:
			return m.updateConfirmFix(msg)
		case screenOutput:
			return m.updateOutput(msg)
		case screenQuiz:
			return m.updateQuiz(msg)
		}
	}
	if m.screen == screenQuiz {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) bodyHeight() int {
	return max(1, m.height-2)
}

func (m *Model) current() *item {
	vis := m.visible()
	if len(vis) == 0 {
		return nil
	}
	m.selected = clamp(m.selected, 0, len(vis)-1)
	return &m.items[vis[m.selected]]
}

func (m *Model) finishQuiz() {
	total := 0
	for _, s := range m.quiz.scores {
		total += s
	}
	hs := m.opts.Store.Hunk(m.quiz.key)
	hs.Quizzed = true
	hs.QuizScore = total / max(1, len(m.quiz.scores))
	m.opts.Store.PutHunk(m.quiz.key, hs)
	m.save()
	m.quiz = quizState{}
	m.textarea.Blur()
	m.screen = screenBrowse
	m.refreshDetail()
}

func (m *Model) layout() {
	h := m.bodyHeight()
	m.viewport.Width = max(10, m.width-m.listWidth()-2)
	if m.zoom {
		m.viewport.Width = m.width
	}
	m.viewport.Height = h
	m.output.Width = m.width
	m.output.Height = h
	m.textarea.SetWidth(min(m.width-4, 110))
	m.textarea.SetHeight(min(8, max(3, h/3)))
	m.refreshDetail()
}

func (m *Model) listWidth() int {
	return max(32, m.width*2/5)
}

func (m Model) onDiffLoaded(msg diffLoadedMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	if msg.err != nil {
		m.errMsg = msg.err.Error()
		return m, nil
	}
	m.errMsg = ""
	m.desc = msg.desc
	fp := rankFingerprint(m.opts)
	m.items = m.items[:0]
	var pending []diff.Hunk
	for _, h := range msg.hunks {
		f, ok := m.opts.Store.Finding(cacheKey(h.Key, fp))
		if ok {
			f.ID = h.ID
		} else {
			f = triage.Heuristic(h)
			pending = append(pending, h)
		}
		m.items = append(m.items, item{finding: f, hunk: h})
	}
	m.selected = 0
	m.refreshDetail()
	if len(pending) == 0 || m.opts.NoAgent {
		return m, nil
	}
	m.busy = fmt.Sprintf("ranking %d hunks with cursor-agent… (heuristic order shown meanwhile)", len(pending))
	return m, rankCmd(m.opts, pending)
}

func (m Model) onQuiz(msg quizMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	if msg.err != nil {
		m.errMsg = "quiz: " + msg.err.Error()
		return m, nil
	}
	m.errMsg = ""
	m.quiz = quizState{key: msg.key, questions: msg.questions}
	m.textarea.Reset()
	m.screen = screenQuiz
	return m, m.textarea.Focus()
}

func (m Model) onRanked(msg rankedMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	m.errMsg = ""
	if msg.err != nil {
		m.errMsg = "rank: " + msg.err.Error()
	}
	key := ""
	if it := m.current(); it != nil {
		key = it.hunk.Key
	}
	for i := range m.items {
		f, ok := msg.findings[m.items[i].hunk.Key]
		if !ok {
			continue
		}
		f.ID = m.items[i].hunk.ID
		m.items[i].finding = f
		m.opts.Store.PutFinding(cacheKey(m.items[i].hunk.Key, msg.fingerprint), f)
	}
	m.save()
	m.selectKey(key)
	m.refreshDetail()
	return m, nil
}

func (m *Model) refreshDetail() {
	it := m.current()
	if it == nil {
		msg := "No changes to review."
		if m.hideReviewed && len(m.items) > 0 {
			msg = "Everything is reviewed. Press H to show reviewed hunks."
		}
		m.viewport.SetContent(msg)
		return
	}
	m.viewport.SetContent(m.renderDetail(*it, m.viewport.Width))
	m.viewport.GotoTop()
}

func (m *Model) save() {
	if err := m.opts.Store.Save(); err != nil {
		m.errMsg = "save state: " + err.Error()
	}
}

func (m *Model) selectKey(key string) {
	for i, idx := range m.visible() {
		if m.items[idx].hunk.Key == key {
			m.selected = i
			return
		}
	}
}

func (m Model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	vis := m.visible()
	m.notice = ""
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "enter", "z":
		m.zoom = !m.zoom
		m.layout()
		return m, nil
	case "esc":
		if m.zoom {
			m.zoom = false
			m.layout()
		}
		return m, nil
	case "y", "Y":
		if it := m.current(); it != nil {
			withDiff := msg.String() == "Y"
			copyToClipboard(findingMarkdown(*it, withDiff))
			m.notice = "copied finding to clipboard"
			if withDiff {
				m.notice = "copied finding + diff to clipboard"
			}
		}
		return m, nil
	case "j", "down":
		m.selected = clamp(m.selected+1, 0, len(vis)-1)
	case "k", "up":
		m.selected = clamp(m.selected-1, 0, len(vis)-1)
	case "g", "home":
		m.selected = 0
	case "G", "end":
		m.selected = max(0, len(vis)-1)
	case "tab", "shift+tab":
		key := m.currentKey()
		step := 1
		if msg.String() == "shift+tab" {
			step = len(sortNames) - 1
		}
		m.sortBy = sortKey((int(m.sortBy) + step) % len(sortNames))
		m.selectKey(key)
	case " ", "space":
		if it := m.current(); it != nil {
			hs := m.opts.Store.Hunk(it.hunk.Key)
			hs.Reviewed = !hs.Reviewed
			m.opts.Store.PutHunk(it.hunk.Key, hs)
			m.save()
			if !m.hideReviewed {
				m.selected = clamp(m.selected+1, 0, len(vis)-1)
			}
		}
	case "H":
		key := m.currentKey()
		m.hideReviewed = !m.hideReviewed
		m.selectKey(key)
	case "R":
		if m.busy == "" {
			m.busy = "reloading diff…"
			return m, loadDiffCmd(m.opts)
		}
	case "r":
		if m.busy == "" && !m.opts.NoAgent && len(m.items) > 0 {
			hunks := make([]diff.Hunk, len(m.items))
			for i, it := range m.items {
				hunks[i] = it.hunk
			}
			m.busy = fmt.Sprintf("re-ranking all %d hunks…", len(hunks))
			return m, rankCmd(m.opts, hunks)
		}
	case "t":
		it := m.current()
		if it == nil || m.busy != "" || m.opts.NoAgent {
			return m, nil
		}
		if m.quiz.key == it.hunk.Key && m.quiz.idx < len(m.quiz.questions) {
			m.screen = screenQuiz
			return m, m.textarea.Focus()
		}
		m.busy = "writing quiz questions…"
		return m, quizCmd(m.opts, *it)
	case "f":
		if m.current() != nil && m.busy == "" && !m.opts.NoAgent {
			m.screen = screenConfirmFix
		}
		return m, nil
	case "ctrl+d", "pgdown":
		m.viewport.HalfViewDown()
		return m, nil
	case "ctrl+u", "pgup":
		m.viewport.HalfViewUp()
		return m, nil
	case "J":
		m.viewport.LineDown(3)
		return m, nil
	case "K":
		m.viewport.LineUp(3)
		return m, nil
	default:
		return m, nil
	}
	m.refreshDetail()
	return m, nil
}

func (m Model) updateConfirmFix(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		it := m.current()
		if it == nil {
			m.screen = screenBrowse
			return m, nil
		}
		m.busy = "cursor-agent is editing files…"
		m.output.SetContent("Working on: " + it.finding.Title)
		m.screen = screenOutput
		return m, fixCmd(m.opts, *it)
	case "n", "esc", "q":
		m.screen = screenBrowse
	}
	return m, nil
}

func (m Model) updateOutput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		if m.busy != "" {
			return m, nil
		}
		m.screen = screenBrowse
		m.busy = "reloading diff…"
		return m, loadDiffCmd(m.opts)
	}
	var cmd tea.Cmd
	m.output, cmd = m.output.Update(msg)
	return m, cmd
}

func (m Model) updateQuiz(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.textarea.Blur()
		m.screen = screenBrowse
		return m, nil
	case "ctrl+s":
		if m.quiz.grade != nil || m.busy != "" {
			return m, nil
		}
		answer := strings.TrimSpace(m.textarea.Value())
		it := m.itemByKey(m.quiz.key)
		if answer == "" || it == nil {
			return m, nil
		}
		m.busy = "grading…"
		return m, gradeCmd(m.opts, *it, m.quiz.questions[m.quiz.idx], answer)
	}
	if m.quiz.grade != nil {
		if s := msg.String(); s == "enter" || s == "n" {
			m.quiz.idx++
			m.quiz.grade = nil
			m.textarea.Reset()
			if m.quiz.idx >= len(m.quiz.questions) {
				m.finishQuiz()
			}
		}
		return m, nil
	}
	if m.busy != "" {
		return m, nil
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m *Model) currentKey() string {
	if it := m.current(); it != nil {
		return it.hunk.Key
	}
	return ""
}

func (m *Model) itemByKey(key string) *item {
	for i := range m.items {
		if m.items[i].hunk.Key == key {
			return &m.items[i]
		}
	}
	return nil
}

// visible returns indices into m.items after filtering and sorting.
func (m *Model) visible() []int {
	idx := make([]int, 0, len(m.items))
	for i, it := range m.items {
		if m.hideReviewed && m.opts.Store.Hunk(it.hunk.Key).Reviewed {
			continue
		}
		idx = append(idx, i)
	}
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := m.items[idx[a]], m.items[idx[b]]
		switch m.sortBy {
		case sortRisk:
			return x.finding.Risk > y.finding.Risk
		case sortImmovable:
			return x.finding.Immovable > y.finding.Immovable
		case sortRegret:
			return x.finding.Regret > y.finding.Regret
		case sortFile:
			if x.hunk.File != y.hunk.File {
				return x.hunk.File < y.hunk.File
			}
			return x.hunk.StartLine < y.hunk.StartLine
		default:
			return x.finding.Score() > y.finding.Score()
		}
	})
	return idx
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
