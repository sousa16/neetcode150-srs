package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWritesDefaultsOnFirstRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := defaults()
	if cfg != want {
		t.Errorf("Load() = %+v, want defaults %+v", cfg, want)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Load() did not write a config file at %s: %v", path, err)
	}
}

func TestLoadHonorsExistingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if err := save(path, Config{StartHour: 6, DailyLimit: 10, DataRepo: "example", Device: "home"}); err != nil {
		t.Fatalf("save() error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := Config{StartHour: 6, DailyLimit: 10, DataRepo: "example", Device: "home"}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadFillsDefaultsForMissingFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	// Only "device" is set; every other field should fall back to defaults().
	if err := os.WriteFile(path, []byte(`{"device":"work"}`), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := defaults()
	want.Device = "work"
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}
