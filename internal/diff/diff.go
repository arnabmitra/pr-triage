package diff

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Mode string

const (
	ModeBoth        Mode = "both"
	ModeBranch      Mode = "branch"
	ModeUncommitted Mode = "uncommitted"
)

var excludes = []string{
	"--", ".",
	":(exclude)go.sum",
	":(exclude)*.lock",
	":(exclude)package-lock.json",
	":(exclude)pnpm-lock.yaml",
}

type Hunk struct {
	Added   int
	Deleted bool
	File    string
	Header  string
	ID      string
	// Key is stable across re-diffs as long as the changed lines are unchanged.
	Key       string
	Lines     []string
	NewFile   bool
	Removed   int
	StartLine int
}

// ChangedText returns only the added and removed lines.
func (h Hunk) ChangedText() string {
	var b strings.Builder
	for _, l := range h.Lines {
		if strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-") {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func GitCommonDir(repo string) (string, error) {
	out, err := git(repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	return strings.TrimSpace(out), err
}

// Load diffs the repo according to mode and returns parsed hunks plus a short description.
func Load(repo string, mode Mode, base string) ([]Hunk, string, error) {
	var args []string
	var desc string
	switch mode {
	case ModeUncommitted:
		args, desc = []string{"HEAD"}, "uncommitted"
	case ModeBranch, ModeBoth:
		ref, mergeBase, err := resolveBase(repo, base)
		if err != nil {
			return nil, "", err
		}
		if mode == ModeBranch {
			args, desc = []string{mergeBase, "HEAD"}, ref+"...HEAD"
		} else {
			args, desc = []string{mergeBase}, ref+"...working tree"
		}
	default:
		return nil, "", fmt.Errorf("unknown mode %q", mode)
	}

	cmd := append([]string{"diff", "--no-color", "--no-ext-diff", "--find-renames", "-U3"}, args...)
	out, err := git(repo, append(cmd, excludes...)...)
	if err != nil {
		return nil, "", err
	}
	hunks := Parse(out)
	files := map[string]bool{}
	for _, h := range hunks {
		files[h.File] = true
	}
	return hunks, fmt.Sprintf("%s · %d hunks in %d files", desc, len(hunks), len(files)), nil
}

// Parse turns unified diff output into hunks.
func Parse(out string) []Hunk {
	var (
		hunks            []Hunk
		cur              *Hunk
		file             string
		newFile, deleted bool
	)
	flush := func() {
		if cur != nil {
			hunks = append(hunks, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			file, newFile, deleted = "", false, false
			if i := strings.LastIndex(line, " b/"); i >= 0 {
				file = line[i+3:]
			}
		case cur == nil && strings.HasPrefix(line, "new file mode"):
			newFile = true
		case cur == nil && strings.HasPrefix(line, "deleted file mode"):
			deleted = true
		case cur == nil && strings.HasPrefix(line, "--- "):
			if deleted {
				file = strings.TrimPrefix(strings.TrimPrefix(line, "--- "), "a/")
			}
		case cur == nil && strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				file = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "@@"):
			flush()
			cur = &Hunk{Deleted: deleted, File: file, Header: line, NewFile: newFile, StartLine: parseStart(line)}
		case cur != nil:
			if line == "" || strings.HasPrefix(line, `\`) {
				continue
			}
			cur.Lines = append(cur.Lines, line)
			switch line[0] {
			case '+':
				cur.Added++
			case '-':
				cur.Removed++
			}
		}
	}
	flush()

	for i := range hunks {
		hunks[i].ID = "h" + strconv.Itoa(i+1)
		sum := sha256.Sum256([]byte(hunks[i].File + "\n" + hunks[i].ChangedText()))
		hunks[i].Key = hex.EncodeToString(sum[:8])
	}
	return hunks
}

func RepoRoot(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repo: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// parseStart extracts the new-file start line from "@@ -a,b +c,d @@".
func parseStart(header string) int {
	i := strings.Index(header, "+")
	if i < 0 {
		return 0
	}
	rest := header[i+1:]
	if j := strings.IndexAny(rest, ", "); j >= 0 {
		rest = rest[:j]
	}
	n, _ := strconv.Atoi(rest)
	return n
}

func resolveBase(repo, base string) (ref, mergeBase string, err error) {
	candidates := []string{base}
	if base == "" {
		candidates = []string{"origin/main", "origin/master", "main", "master"}
	}
	for _, c := range candidates {
		if _, err := git(repo, "rev-parse", "--verify", "--quiet", c+"^{commit}"); err != nil {
			continue
		}
		mb, err := git(repo, "merge-base", c, "HEAD")
		if err != nil {
			return "", "", err
		}
		return c, strings.TrimSpace(mb), nil
	}
	return "", "", fmt.Errorf("no base ref found (tried %s); pass --base", strings.Join(candidates, ", "))
}
