package skills

import (
	"embed"
	"errors"
	"os"
	"path/filepath"
)

const (
	Fix   = "fix"
	Grade = "grade"
	Quiz  = "quiz"
	Rank  = "rank"
)

var Names = []string{Fix, Grade, Quiz, Rank}

//go:embed defaults/*.md
var defaults embed.FS

// Load returns the skill text, preferring <repo>/.pr-triage/skills, then ~/.config/pr-triage/skills,
// then the built-in default. It is read on every call so edits apply without restarting.
func Load(repo, name string) (text, source string, err error) {
	paths := []string{filepath.Join(repo, ".pr-triage", "skills", name+".md")}
	if dir, err := UserDir(); err == nil {
		paths = append(paths, filepath.Join(dir, name+".md"))
	}
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			return string(b), p, nil
		}
	}
	b, err := defaults.ReadFile("defaults/" + name + ".md")
	return string(b), "built-in", err
}

func UserDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "pr-triage", "skills"), nil
}

// WriteDefaults copies built-in skills into dir without overwriting existing files.
func WriteDefaults(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var written []string
	for _, name := range Names {
		dst := filepath.Join(dir, name+".md")
		if _, err := os.Stat(dst); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return written, err
		}
		b, err := defaults.ReadFile("defaults/" + name + ".md")
		if err != nil {
			return written, err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return written, err
		}
		written = append(written, dst)
	}
	return written, nil
}
