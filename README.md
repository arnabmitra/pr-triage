# pr-triage

A terminal UI that puts the riskiest parts of your changeset first, then quizzes you on them.

Each hunk gets three 0–10 scores from a `cursor-agent` sub-agent:

- **immovable**: how hard it is to change after it ships (migrations, public APIs, contracts, wire formats).
- **regret**: how bad it is, and how late you'd find out, if it's subtly wrong.
- **risk**: how likely it is to be wrong right now.

They combine into one priority: `0.4·risk + 0.35·immovable + 0.25·regret`. Heuristic scores (marked `~`) appear instantly, and agent scores replace them as they arrive.

## Install

```sh
go install github.com/arnabmitra/pr-triage@latest   # or: go build -o pr-triage .
```

Requires `git` and the `cursor-agent` CLI, signed in.

## Use

```sh
pr-triage                         # branch vs origin/main plus uncommitted changes (default)
pr-triage -mode branch            # committed changes only
pr-triage -mode uncommitted       # working tree vs HEAD only
pr-triage -base develop -model claude-opus-4-8
pr-triage -no-agent               # heuristics only, no quiz or fix
```

| Key | Action |
| --- | --- |
| `j`/`k` | move |
| `space` | toggle reviewed |
| `t` | quiz me on this hunk (`ctrl+s` submits, `enter` goes to the next question) |
| `f` | let the agent fix this finding (asks first; runs with `--force`) |
| `tab` | cycle sort: score, risk, immovable, regret, file |
| `H` | hide reviewed hunks |
| `J`/`K`, `ctrl+d`/`ctrl+u` | scroll the diff |
| `r` / `R` | re-rank everything / reload the diff |

Progress (reviewed, quiz scores) and agent rankings are saved in `.git/pr-triage/state.json`. They're keyed by each hunk's changed lines, so they survive rebases and unrelated edits. Changing the rank skill or the model invalidates the cached rankings.

## Tuning

```sh
pr-triage init-skills   # copies the built-in skills to ~/.config/pr-triage/skills/
```

Edit `rank.md`, `quiz.md`, `grade.md`, or `fix.md`. For per-repo overrides, put files in `<repo>/.pr-triage/skills/`. Skills are re-read on every call, so you don't need to restart. The JSON output format is enforced in code, so edits to a skill can't break parsing.

Ranking runs the agent in read-only `ask` mode with the repo as its workspace, so it can read callers. That makes it thorough but slow on large diffs. Pass a faster `-model` if you want speed over depth.
