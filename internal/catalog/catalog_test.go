package catalog

import "testing"

func TestLoadReturnsAllProblems(t *testing.T) {
	problems := Load()

	if len(problems) != 150 {
		t.Errorf("len(problems) = %d, want 150", len(problems))
	}
}

func TestLoadFieldsAndUniqueness(t *testing.T) {
	problems := Load()

	seen := make(map[string]bool, len(problems))
	for _, p := range problems {
		if p.ID == "" {
			t.Errorf("problem has empty ID: %+v", p)
		}
		if p.Title == "" {
			t.Errorf("problem %s has empty Title", p.ID)
		}
		if p.Topic == "" {
			t.Errorf("problem %s has empty Topic", p.ID)
		}
		if p.Priority != "core" && p.Priority != "secondary" {
			t.Errorf("problem %s has invalid Priority %q", p.ID, p.Priority)
		}
		if seen[p.ID] {
			t.Errorf("duplicate problem id %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestLoadPriorityCounts(t *testing.T) {
	problems := Load()

	var core, secondary int
	for _, p := range problems {
		switch p.Priority {
		case "core":
			core++
		case "secondary":
			secondary++
		}
	}

	if core != 129 {
		t.Errorf("core count = %d, want 129", core)
	}
	if secondary != 21 {
		t.Errorf("secondary count = %d, want 21", secondary)
	}
}
