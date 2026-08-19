package store

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sousa16/neetcode150-srs/internal/config"
	"github.com/sousa16/neetcode150-srs/internal/srs"
)

// initBareRepo creates a bare git repo in a temp dir to stand in for the
// real GitHub remote, so sync tests never touch the network.
func initBareRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	return dir
}

// writeFakeRemoteConfig points the current $HOME's config at a local bare
// repo instead of the real remote, so sync tests never touch the network.
// $HOME must already be set (via t.Setenv) before calling this.
func writeFakeRemoteConfig(t *testing.T, remote string) {
	t.Helper()

	path, err := config.Path()
	if err != nil {
		t.Fatalf("config.Path() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	data, err := json.Marshal(config.Config{StartHour: 4, DailyLimit: 5, DataRepo: remote})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

// setGitIdentity configures user.name/user.email locally in dir, so commits
// work even in an environment with no global git config.
func setGitIdentity(t *testing.T, dir string) {
	t.Helper()

	for _, args := range [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func TestPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}

	want := filepath.Join(home, ".local", "share", "neetcode-srs", "reviews.jsonl")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestAppendAndLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	e1, err := Append(time.Now(), "two-sum", srs.GradeGood, 18, "work")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	e2, err := Append(time.Now(), "valid-anagram", srs.GradeStudy, 0, "")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if e1.UID == "" || e2.UID == "" {
		t.Fatal("Append() left UID empty")
	}
	if e1.UID == e2.UID {
		t.Error("Append() produced duplicate UIDs")
	}
	if e1.At.Location() != time.UTC {
		t.Errorf("At.Location() = %v, want UTC", e1.At.Location())
	}

	events, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].ID != "two-sum" || events[0].Grade != srs.GradeGood || events[0].Mins != 18 || events[0].Device != "work" {
		t.Errorf("events[0] = %+v, want id=two-sum grade=good mins=18 device=work", events[0])
	}
	if events[1].ID != "valid-anagram" || events[1].Grade != srs.GradeStudy || events[1].Device != "" {
		t.Errorf("events[1] = %+v, want id=valid-anagram grade=study device=\"\"", events[1])
	}
}

func TestLoadMissingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	events, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if events != nil {
		t.Errorf("Load() = %v, want nil for missing file", events)
	}
}

func TestFold(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.AddDate(0, 0, 5)

	events := []Event{
		{UID: "b", ID: "two-sum", At: t1, Grade: srs.GradeGood},
		{UID: "a", ID: "two-sum", At: t0, Grade: srs.GradeStudy},
		{UID: "a", ID: "two-sum", At: t0, Grade: srs.GradeEasy}, // duplicate uid, must be ignored
	}

	cards := Fold(events)

	card, ok := cards["two-sum"]
	if !ok {
		t.Fatal("Fold() missing card for two-sum")
	}
	if card.State != srs.StateReview {
		t.Errorf("State = %v, want %v", card.State, srs.StateReview)
	}
	if card.Interval != 3 {
		t.Errorf("Interval = %d, want 3", card.Interval)
	}
}

func TestFoldSkipsInvalidHistoricalEvent(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.AddDate(0, 0, 5)

	events := []Event{
		{UID: "a", ID: "two-sum", At: t0, Grade: srs.GradeGood},
		// invalid under current rules (study on a review-state card), e.g.
		// from a log written before this validation existed
		{UID: "b", ID: "two-sum", At: t1, Grade: srs.GradeStudy},
	}

	cards := Fold(events)

	card, ok := cards["two-sum"]
	if !ok {
		t.Fatal("Fold() missing card for two-sum")
	}
	if card.State != srs.StateReview {
		t.Errorf("State = %v, want %v (invalid event should be a no-op)", card.State, srs.StateReview)
	}
	if card.Interval != 3 {
		t.Errorf("Interval = %d, want 3 (invalid event should be a no-op)", card.Interval)
	}
}

func TestPullClonesOnFirstRun(t *testing.T) {
	remote := initBareRepo(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFakeRemoteConfig(t, remote)

	if err := Pull(); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}

	dir, err := dataDir()
	if err != nil {
		t.Fatalf("dataDir() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("Pull() did not leave a cloned repo at %s: %v", dir, err)
	}
}

func TestPushCommitsAndPushesToRemote(t *testing.T) {
	remote := initBareRepo(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFakeRemoteConfig(t, remote)

	if err := Pull(); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}

	dir, err := dataDir()
	if err != nil {
		t.Fatalf("dataDir() error = %v", err)
	}
	setGitIdentity(t, dir)

	if _, err := Append(time.Now(), "two-sum", srs.GradeGood, 12, "work"); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if err := Push("log two-sum: good"); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// Verify the commit actually landed on the remote by cloning it fresh
	// into a separate directory, independent of the working copy above.
	verifyDir := t.TempDir()
	if out, err := exec.Command("git", "clone", remote, verifyDir).CombinedOutput(); err != nil {
		t.Fatalf("verification clone failed: %v: %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(verifyDir, "reviews.jsonl"))
	if err != nil {
		t.Fatalf("reading pushed reviews.jsonl: %v", err)
	}
	if !strings.Contains(string(data), "two-sum") {
		t.Errorf("pushed reviews.jsonl missing expected event, got: %q", data)
	}
}

func TestSyncBetweenTwoDevices(t *testing.T) {
	remote := initBareRepo(t)

	deviceA := t.TempDir()
	deviceB := t.TempDir()

	// Device A: clone, log a review, push.
	t.Setenv("HOME", deviceA)
	writeFakeRemoteConfig(t, remote)
	if err := Pull(); err != nil {
		t.Fatalf("device A Pull() error = %v", err)
	}
	dirA, err := dataDir()
	if err != nil {
		t.Fatalf("dataDir() error = %v", err)
	}
	setGitIdentity(t, dirA)
	if _, err := Append(time.Now(), "two-sum", srs.GradeGood, 12, "work"); err != nil {
		t.Fatalf("device A Append() error = %v", err)
	}
	if err := Push("device A: log two-sum"); err != nil {
		t.Fatalf("device A Push() error = %v", err)
	}

	// Device B: clone fresh, should see device A's review immediately.
	t.Setenv("HOME", deviceB)
	writeFakeRemoteConfig(t, remote)
	if err := Pull(); err != nil {
		t.Fatalf("device B Pull() error = %v", err)
	}

	events, err := Load()
	if err != nil {
		t.Fatalf("device B Load() error = %v", err)
	}
	if len(events) != 1 || events[0].ID != "two-sum" {
		t.Fatalf("device B events = %+v, want device A's two-sum review", events)
	}

	// Device B logs its own review and pushes back.
	dirB, err := dataDir()
	if err != nil {
		t.Fatalf("dataDir() error = %v", err)
	}
	setGitIdentity(t, dirB)
	if _, err := Append(time.Now(), "valid-anagram", srs.GradeEasy, 5, "home"); err != nil {
		t.Fatalf("device B Append() error = %v", err)
	}
	if err := Push("device B: log valid-anagram"); err != nil {
		t.Fatalf("device B Push() error = %v", err)
	}

	// Device A pulls again and should now see both reviews.
	t.Setenv("HOME", deviceA)
	if err := Pull(); err != nil {
		t.Fatalf("device A second Pull() error = %v", err)
	}
	events, err = Load()
	if err != nil {
		t.Fatalf("device A second Load() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("device A events after resync = %+v, want 2 events", events)
	}
}

func TestFoldEmpty(t *testing.T) {
	cards := Fold(nil)
	if len(cards) != 0 {
		t.Errorf("Fold(nil) = %v, want empty map", cards)
	}
}

func TestFoldSkipsVoidedEvent(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.AddDate(0, 0, 1)

	events := []Event{
		{UID: "a", ID: "two-sum", At: t0, Grade: srs.GradeGood},
		{UID: "b", ID: "two-sum", At: t1, Grade: srs.GradeHard, Voids: "a"},
	}

	cards := Fold(events)
	if _, ok := cards["two-sum"]; ok {
		t.Errorf("Fold() = %v, want no card once its only event is voided", cards)
	}
}

func TestFoldReplaysAroundVoidedMiddleEvent(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.AddDate(0, 0, 1)
	t2 := t0.AddDate(0, 0, 2)

	events := []Event{
		{UID: "a", ID: "two-sum", At: t0, Grade: srs.GradeGood},
		{UID: "b", ID: "two-sum", At: t1, Grade: srs.GradeAgain}, // logged by mistake
		{UID: "c", ID: "two-sum", At: t2, Grade: srs.GradeEasy, Voids: "b"},
	}

	cards := Fold(events)
	card, ok := cards["two-sum"]
	if !ok {
		t.Fatal("Fold() missing card for two-sum")
	}
	// With b voided, only a (good, interval 3) feeds scheduling; ease
	// should never have taken the "again" lapse penalty.
	if card.Lapses != 0 {
		t.Errorf("Lapses = %d, want 0 (the again grade was voided)", card.Lapses)
	}
}

func TestActiveEventsExcludesTombstonesAndVoidedTargets(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.AddDate(0, 0, 1)
	t2 := t0.AddDate(0, 0, 2)

	events := []Event{
		{UID: "a", ID: "two-sum", At: t0, Grade: srs.GradeGood},
		{UID: "b", ID: "valid-anagram", At: t1, Grade: srs.GradeHard},
		{UID: "c", ID: "valid-anagram", At: t2, Grade: srs.GradeHard, Voids: "b"},
	}

	active := ActiveEvents(events)
	if len(active) != 1 || active[0].UID != "a" {
		t.Errorf("ActiveEvents() = %+v, want only event a", active)
	}
}

func TestVoidAppendsTombstone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target, err := Append(time.Now(), "two-sum", srs.GradeGood, 18, "work")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	voidEvent, err := Void(time.Now(), target, "work")
	if err != nil {
		t.Fatalf("Void() error = %v", err)
	}
	if voidEvent.Voids != target.UID {
		t.Errorf("voidEvent.Voids = %q, want %q", voidEvent.Voids, target.UID)
	}
	if voidEvent.UID == target.UID {
		t.Error("Void() reused the target's UID instead of minting a new one")
	}

	events, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2 (original event untouched, tombstone appended)", len(events))
	}
	if events[0] != target {
		t.Errorf("events[0] = %+v, want the untouched original %+v", events[0], target)
	}

	if _, ok := Fold(events)["two-sum"]; ok {
		t.Error("Fold() still has a card for two-sum after its only event was voided")
	}
}
