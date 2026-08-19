package store

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sousa16/neetcode150-srs/internal/config"
	"github.com/sousa16/neetcode150-srs/internal/srs"
)

const gitTimeout = 10 * time.Second

type Event struct {
	UID    string    `json:"uid"`
	ID     string    `json:"id"`
	At     time.Time `json:"at"`
	Grade  srs.Grade `json:"grade"`
	Mins   int       `json:"mins,omitempty"`
	Device string    `json:"device,omitempty"`
	// Voids holds the UID of an earlier event this one cancels. The log is
	// append-only (see DESIGN.md), so undo can't delete or rewrite a line —
	// it appends a tombstone instead, and Fold skips whatever UID it names.
	Voids string `json:"voids,omitempty"`
}

func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "neetcode-srs"), nil
}

func Path() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reviews.jsonl"), nil
}

func Append(now time.Time, id string, grade srs.Grade, mins int, device string) (Event, error) {
	e := Event{
		UID:    newUID(),
		ID:     id,
		At:     now.UTC(),
		Grade:  grade,
		Mins:   mins,
		Device: device,
	}
	if err := appendEvent(e); err != nil {
		return Event{}, err
	}
	return e, nil
}

// Void appends a tombstone event canceling target: Fold will replay the log
// as if target's UID were never there. target itself is never touched — the
// log stays append-only so sync (merge=union) can never conflict.
func Void(now time.Time, target Event, device string) (Event, error) {
	e := Event{
		UID:    newUID(),
		ID:     target.ID,
		At:     now.UTC(),
		Grade:  target.Grade,
		Device: device,
		Voids:  target.UID,
	}
	if err := appendEvent(e); err != nil {
		return Event{}, err
	}
	return e, nil
}

func appendEvent(e Event) error {
	path, err := Path()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	_, err = f.Write(line)
	return err
}

func Load() ([]Event, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

// ActiveEvents returns the events that still count toward scheduling:
// deduplicated by UID, sorted oldest-first, with tombstones and whatever
// they voided removed. It's the single source of truth Fold and the undo
// command both build on.
func ActiveEvents(events []Event) []Event {
	sorted := make([]Event, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].At.Before(sorted[j].At)
	})

	voided := make(map[string]bool)
	for _, e := range sorted {
		if e.Voids != "" {
			voided[e.Voids] = true
		}
	}

	var active []Event
	seen := make(map[string]bool)
	for _, e := range sorted {
		if seen[e.UID] {
			continue
		}
		seen[e.UID] = true

		if e.Voids != "" || voided[e.UID] {
			continue
		}
		active = append(active, e)
	}

	return active
}

func Fold(events []Event) map[string]srs.Card {
	cards := make(map[string]srs.Card)

	for _, e := range ActiveEvents(events) {
		card, ok := cards[e.ID]
		if !ok {
			card = srs.NewCard(e.ID)
		}

		// A pre-existing log can contain an event that's invalid under the
		// current rules (e.g. logged before ErrStudyNotAllowed existed). The
		// log itself is never rewritten, so replay just leaves the card
		// unchanged for that one event rather than failing the whole fold.
		next, err := srs.Schedule(e.At, card, e.Grade)
		if err != nil {
			continue
		}
		cards[e.ID] = next
	}

	return cards
}

// Pull brings the local data repo up to date before a read. If the repo
// hasn't been cloned yet (first run on this machine), it clones instead.
// Per DESIGN.md, sync failures are meant to be non-fatal — the caller
// decides whether to warn and continue, not this function.
func Pull() error {
	dir, err := dataDir()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		return cloneRepo(dir)
	}

	return runGit(dir, "pull", "--rebase")
}

// Push commits whatever local changes exist (expected to be a freshly
// appended review) and pushes them to the data repo's remote.
func Push(message string) error {
	dir, err := dataDir()
	if err != nil {
		return err
	}

	if err := runGit(dir, "add", "-A"); err != nil {
		return err
	}
	if err := runGit(dir, "commit", "-m", message); err != nil {
		return err
	}
	return runGit(dir, "push")
}

func cloneRepo(dir string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return err
	}
	return runGit(filepath.Dir(dir), "clone", cfg.DataRepo, dir)
}

func runGit(dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func newUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
