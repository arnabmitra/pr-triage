package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arnabmitra/pr-triage/internal/triage"
)

type HunkState struct {
	QuizScore int  `json:"quiz_score"`
	Quizzed   bool `json:"quizzed"`
	Reviewed  bool `json:"reviewed"`
}

type State struct {
	// Findings caches agent rankings keyed by hunk key + rank-skill fingerprint.
	Findings map[string]triage.Finding `json:"findings"`
	Hunks    map[string]HunkState      `json:"hunks"`
}

// Store persists review progress. It is not goroutine-safe; only touch it from the UI loop.
type Store struct {
	path  string
	state State
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, state: State{Findings: map[string]triage.Finding{}, Hunks: map[string]HunkState{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.state); err != nil {
		return nil, fmt.Errorf("corrupt state file %s: %w", path, err)
	}
	if s.state.Findings == nil {
		s.state.Findings = map[string]triage.Finding{}
	}
	if s.state.Hunks == nil {
		s.state.Hunks = map[string]HunkState{}
	}
	return s, nil
}

func (s *Store) Finding(key string) (triage.Finding, bool) {
	f, ok := s.state.Findings[key]
	return f, ok
}

func (s *Store) Hunk(key string) HunkState {
	return s.state.Hunks[key]
}

func (s *Store) PutFinding(key string, f triage.Finding) {
	s.state.Findings[key] = f
}

func (s *Store) PutHunk(key string, hs HunkState) {
	s.state.Hunks[key] = hs
}

func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
