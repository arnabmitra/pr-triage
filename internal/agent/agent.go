package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/arnabmitra/pr-triage/internal/diff"
	"github.com/arnabmitra/pr-triage/internal/triage"
)

const (
	maxBatchBytes  = 60_000
	maxConcurrency = 3
	maxHunkLines   = 150
	gradeContract  = `## Output contract (do not deviate)
Return ONLY a JSON object, no prose, no code fences:
{"score":0-100,"verdict":"got it|partial|missed","feedback":"2-3 sentences","answer":"a model answer in 1-3 sentences"}
Do not edit any files.`
	quizContract = `## Output contract (do not deviate)
Return ONLY a JSON object, no prose, no code fences:
{"questions":[{"q":"the question","expect":"key points a correct answer must contain"}]}
Do not edit any files.`
	rankContract = `## Output contract (do not deviate)
Return ONLY a JSON object, no prose, no code fences:
{"findings":[{"id":"h1","immovable":0-10,"regret":0-10,"risk":0-10,"title":"<=70 chars","why":"1-3 sentences","check":"one concrete thing to verify"}]}
Include every hunk id listed below exactly once. Do not edit any files.`
)

// Client shells out to the cursor-agent CLI in non-interactive mode.
type Client struct {
	Bin   string
	Model string
	Repo  string
}

type Grade struct {
	Answer   string `json:"answer"`
	Feedback string `json:"feedback"`
	Score    int    `json:"score"`
	Verdict  string `json:"verdict"`
}

type Question struct {
	Expect string `json:"expect"`
	Q      string `json:"q"`
}

// Fix lets the agent edit files in the repo to address a finding.
func (c *Client) Fix(ctx context.Context, skill string, h diff.Hunk, f triage.Finding) (string, error) {
	prompt := fmt.Sprintf("%s\n\n## Finding\n%s\n\nWhy: %s\nCheck: %s\n\n## Hunk\n%s", skill, f.Title, f.Why, f.Check, formatHunk(h))
	out, err := c.run(ctx, prompt, true)
	return stripRouting(out), err
}

func (c *Client) Grade(ctx context.Context, skill string, h diff.Hunk, q Question, answer string) (Grade, error) {
	prompt := fmt.Sprintf("%s\n\n%s\n\n## Hunk\n%s\n## Question\n%s\n\n## Expected key points\n%s\n\n## Reviewer's answer\n%s",
		skill, gradeContract, formatHunk(h), q.Q, q.Expect, answer)
	out, err := c.run(ctx, prompt, false)
	if err != nil {
		return Grade{}, err
	}
	var g Grade
	return g, extractJSON(out, &g)
}

func (c *Client) Quiz(ctx context.Context, skill string, h diff.Hunk, f triage.Finding) ([]Question, error) {
	prompt := fmt.Sprintf("%s\n\n%s\n\n## Why this hunk was flagged\n%s: %s\n\n## Hunk\n%s", skill, quizContract, f.Title, f.Why, formatHunk(h))
	out, err := c.run(ctx, prompt, false)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Questions []Question `json:"questions"`
	}
	if err := extractJSON(out, &resp); err != nil {
		return nil, err
	}
	if len(resp.Questions) == 0 {
		return nil, errors.New("agent returned no questions")
	}
	return resp.Questions, nil
}

// Rank scores hunks in parallel batches and returns findings keyed by hunk ID.
// Partial results are returned alongside an error if some batches fail.
func (c *Client) Rank(ctx context.Context, skill string, hunks []diff.Hunk) (map[string]triage.Finding, error) {
	batches := batch(hunks)
	results := make([][]triage.Finding, len(batches))
	errs := make([]error, len(batches))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for i, b := range batches {
		wg.Add(1)
		go func(i int, b []diff.Hunk) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], errs[i] = c.rankBatch(ctx, skill, b)
		}(i, b)
	}
	wg.Wait()

	out := map[string]triage.Finding{}
	for _, fs := range results {
		for _, f := range fs {
			f.Source = triage.SourceAgent
			out[f.ID] = f
		}
	}
	return out, errors.Join(errs...)
}

func (c *Client) rankBatch(ctx context.Context, skill string, hunks []diff.Hunk) ([]triage.Finding, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n%s\n\n## Hunks\n", skill, rankContract)
	for _, h := range hunks {
		b.WriteString(formatHunk(h))
	}
	out, err := c.run(ctx, b.String(), false)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Findings []triage.Finding `json:"findings"`
	}
	return resp.Findings, extractJSON(out, &resp)
}

func (c *Client) run(ctx context.Context, prompt string, write bool) (string, error) {
	args := []string{"-p", "--trust", "--output-format", "json", "--workspace", c.Repo}
	if write {
		args = append(args, "--force")
	} else {
		args = append(args, "--mode", "ask")
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Dir = c.Repo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", c.Bin, err, strings.TrimSpace(stderr.String()))
	}
	var env struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return "", fmt.Errorf("decode %s output: %w", c.Bin, err)
	}
	if env.IsError {
		return "", fmt.Errorf("%s: %s", c.Bin, env.Result)
	}
	return env.Result, nil
}

func batch(hunks []diff.Hunk) [][]diff.Hunk {
	var out [][]diff.Hunk
	var cur []diff.Hunk
	size := 0
	for _, h := range hunks {
		n := len(formatHunk(h))
		if len(cur) > 0 && size+n > maxBatchBytes {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, h)
		size += n
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// extractJSON pulls the first JSON value out of agent text that may include routing notes or fences.
func extractJSON(s string, v any) error {
	if i := strings.Index(s, "```json"); i >= 0 {
		rest := s[i+len("```json"):]
		if j := strings.Index(rest, "```"); j >= 0 && json.Unmarshal([]byte(rest[:j]), v) == nil {
			return nil
		}
	}
	start := strings.IndexAny(s, "{[")
	end := strings.LastIndexAny(s, "}]")
	if start < 0 || end < start {
		return fmt.Errorf("no JSON in agent output: %.200q", s)
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), v); err != nil {
		return fmt.Errorf("parse agent JSON: %w", err)
	}
	return nil
}

func formatHunk(h diff.Hunk) string {
	lines := h.Lines
	note := ""
	if len(lines) > maxHunkLines {
		note = fmt.Sprintf("\n… %d more lines truncated", len(lines)-maxHunkLines)
		lines = lines[:maxHunkLines]
	}
	return fmt.Sprintf("### %s — %s\n```diff\n%s\n%s%s\n```\n\n", h.ID, h.File, h.Header, strings.Join(lines, "\n"), note)
}

func stripRouting(s string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "> Auto routed") {
			continue
		}
		keep = append(keep, l)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}
