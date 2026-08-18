package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sousa16/neetcode150-srs/internal/config"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "nc-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "nc")
	build := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := build.CombinedOutput(); err != nil {
		panic("build failed: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

func run(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	// nc reads/writes review history under $HOME via internal/store; give
	// every invocation an isolated fake home so tests never touch the real
	// user's ~/.local/share/neetcode-srs and never leak state between tests.
	return runIn(t, t.TempDir(), args...)
}

// runIn is like run but lets the caller reuse the same fake $HOME across
// multiple invocations, for tests that need state (e.g. a prior "log") to
// persist between one command and the next.
func runIn(t *testing.T, home string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run binary: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}

	return outBuf.String(), errBuf.String(), exitCode
}

func TestLogRejectsUnknownID(t *testing.T) {
	_, stderr, code := run(t, "log", "not-a-real-problem", "good")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "unknown problem id") {
		t.Errorf("stderr = %q, want mention of unknown problem id", stderr)
	}
}

func TestLogRejectsInvalidGrade(t *testing.T) {
	_, stderr, code := run(t, "log", "two-sum", "bogus")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "invalid grade") {
		t.Errorf("stderr = %q, want mention of invalid grade", stderr)
	}
}

func TestLogRejectsMissingArgs(t *testing.T) {
	_, stderr, code := run(t, "log", "two-sum")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "usage: nc log") {
		t.Errorf("stderr = %q, want usage message", stderr)
	}
}

func TestLogAcceptsValidInput(t *testing.T) {
	_, stderr, code := run(t, "log", "two-sum", "good", "--no-sync")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
}

func TestLogRejectsStudyOnReviewCard(t *testing.T) {
	home := t.TempDir()

	_, stderr, code := runIn(t, home, "log", "two-sum", "good", "--no-sync")
	if code != 0 {
		t.Fatalf("setup log failed: exit %d, stderr %s", code, stderr)
	}

	_, stderr, code = runIn(t, home, "log", "two-sum", "study", "--no-sync")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "study") {
		t.Errorf("stderr = %q, want mention of study not being allowed", stderr)
	}
}

func TestLogRejectsInvalidMins(t *testing.T) {
	_, stderr, code := run(t, "log", "two-sum", "good", "--mins", "-5")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Invalid number of minutes") {
		t.Errorf("stderr = %q, want mention of invalid minutes", stderr)
	}
}

func TestDueRejectsInvalidLimit(t *testing.T) {
	_, stderr, code := run(t, "due", "--limit", "-1")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Invalid limit") {
		t.Errorf("stderr = %q, want mention of invalid limit", stderr)
	}
}

func TestDueAcceptsValidLimit(t *testing.T) {
	_, stderr, code := run(t, "due", "--limit", "3", "--no-sync")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
}

func TestListPrintsCatalog(t *testing.T) {
	stdout, stderr, code := run(t, "list", "--no-sync")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "two-sum") {
		t.Errorf("stdout missing expected problem id, got: %q", stdout)
	}
	if lines := strings.Count(stdout, "\n"); lines != 150 {
		t.Errorf("stdout has %d lines, want 150", lines)
	}
}

// writeConfig seeds a config.json into home so a test can control cfg
// values (like daily_limit) that nc reads on startup.
func writeConfig(t *testing.T, home string, cfg config.Config) {
	t.Helper()

	// config.Path() reads $HOME itself, so temporarily point it at home
	// for the duration of this call.
	t.Setenv("HOME", home)
	path, err := config.Path()
	if err != nil {
		t.Fatalf("config.Path() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestDueUsesConfiguredDailyLimit(t *testing.T) {
	home := t.TempDir()
	writeConfig(t, home, config.Config{StartHour: 4, DailyLimit: 7, DataRepo: "unused"})

	// --limit's default comes from config, so -h's auto-generated usage
	// text is a clean way to observe it without needing an actual due item.
	stdout, stderr, code := runIn(t, home, "due", "-h", "--no-sync")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout+stderr, "(default 7)") {
		t.Errorf("usage output = %q, want it to reflect configured daily_limit 7", stdout+stderr)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, stderr, code := run(t, "bogus")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Command not recognized") {
		t.Errorf("stderr = %q, want 'Command not recognized'", stderr)
	}
}

func TestListFiltersByTopic(t *testing.T) {
	stdout, stderr, code := run(t, "list", "--topic", "Arrays & Hashing", "--no-sync")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "two-sum") {
		t.Errorf("stdout missing expected problem in topic, got: %q", stdout)
	}
	if strings.Contains(stdout, "\nvalid-parentheses") || strings.HasPrefix(stdout, "valid-parentheses") {
		t.Errorf("stdout leaked a problem outside the topic filter: %q", stdout)
	}
}

func TestListFiltersByState(t *testing.T) {
	stdoutNew, stderr, code := run(t, "list", "--state", "new", "--no-sync")
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if lines := strings.Count(stdoutNew, "\n"); lines != 150 {
		t.Errorf("--state new: got %d lines, want 150 (no reviews recorded yet)", lines)
	}

	stdoutReview, _, _ := run(t, "list", "--state", "review", "--no-sync")
	if stdoutReview != "" {
		t.Errorf("--state review: want empty output with no review log, got: %q", stdoutReview)
	}
}

func TestListRejectsInvalidState(t *testing.T) {
	_, stderr, code := run(t, "list", "--state", "bogus")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "invalid state") {
		t.Errorf("stderr = %q, want mention of invalid state", stderr)
	}
}
