package srs

import (
	"errors"
	"testing"
	"time"
)

func TestSchedule(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		card         Card
		grade        Grade
		wantState    State
		wantInterval int
		wantEase     float64
		wantLapses   int
	}{
		{"new+study", Card{State: StateNew, Ease: 2.5}, GradeStudy, StateLearning, 1, 2.5, 0},
		{"new+again", Card{State: StateNew, Ease: 2.5}, GradeAgain, StateReview, 1, 2.5, 0},
		{"new+hard", Card{State: StateNew, Ease: 2.5}, GradeHard, StateReview, 2, 2.5, 0},
		{"new+good", Card{State: StateNew, Ease: 2.5}, GradeGood, StateReview, 3, 2.5, 0},
		{"new+easy", Card{State: StateNew, Ease: 2.5}, GradeEasy, StateReview, 5, 2.5, 0},

		{"learning+study", Card{State: StateLearning, Ease: 2.5}, GradeStudy, StateLearning, 1, 2.5, 0},
		{"learning+again", Card{State: StateLearning, Ease: 2.5}, GradeAgain, StateLearning, 1, 2.5, 0},
		{"learning+hard", Card{State: StateLearning, Ease: 2.5}, GradeHard, StateReview, 2, 2.5, 0},
		{"learning+good", Card{State: StateLearning, Ease: 2.5}, GradeGood, StateReview, 3, 2.5, 0},
		{"learning+easy", Card{State: StateLearning, Ease: 2.5}, GradeEasy, StateReview, 5, 2.5, 0},

		{"review+again", Card{State: StateReview, Interval: 10, Ease: 2.5, Lapses: 1}, GradeAgain, StateLearning, 1, 2.3, 2},
		{"review+hard", Card{State: StateReview, Interval: 10, Ease: 2.5}, GradeHard, StateReview, 12, 2.35, 0},
		{"review+good", Card{State: StateReview, Interval: 10, Ease: 2.0}, GradeGood, StateReview, 20, 2.0, 0},
		{"review+easy", Card{State: StateReview, Interval: 10, Ease: 2.0}, GradeEasy, StateReview, 26, 2.15, 0},

		{"review+again clamps ease at floor", Card{State: StateReview, Interval: 5, Ease: 1.4}, GradeAgain, StateLearning, 1, 1.3, 1},
		{"review+easy clamps ease at ceiling", Card{State: StateReview, Interval: 5, Ease: 2.65}, GradeEasy, StateReview, 17, 2.7, 0},
		{"review+good clamps interval at max", Card{State: StateReview, Interval: 100, Ease: 2.5}, GradeGood, StateReview, 120, 2.5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Schedule(now, tt.card, tt.grade)
			if err != nil {
				t.Fatalf("Schedule() error = %v, want nil", err)
			}

			if got.State != tt.wantState {
				t.Errorf("State = %v, want %v", got.State, tt.wantState)
			}
			if got.Interval != tt.wantInterval {
				t.Errorf("Interval = %v, want %v", got.Interval, tt.wantInterval)
			}
			if got.Ease != tt.wantEase {
				t.Errorf("Ease = %v, want %v", got.Ease, tt.wantEase)
			}
			if got.Lapses != tt.wantLapses {
				t.Errorf("Lapses = %v, want %v", got.Lapses, tt.wantLapses)
			}

			wantDue := now.AddDate(0, 0, tt.wantInterval)
			if !got.Due.Equal(wantDue) {
				t.Errorf("Due = %v, want %v", got.Due, wantDue)
			}
		})
	}
}

func TestScheduleRejectsStudyOnReview(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	card := Card{State: StateReview, Interval: 10, Ease: 2.5, Lapses: 1}

	got, err := Schedule(now, card, GradeStudy)

	if !errors.Is(err, ErrStudyNotAllowed) {
		t.Fatalf("err = %v, want ErrStudyNotAllowed", err)
	}
	if got != card {
		t.Errorf("Schedule() returned %+v on error, want unchanged input %+v", got, card)
	}
}

func TestNewCard(t *testing.T) {
	c := NewCard("two-sum")

	if c.ID != "two-sum" {
		t.Errorf("ID = %v, want two-sum", c.ID)
	}
	if c.State != StateNew {
		t.Errorf("State = %v, want %v", c.State, StateNew)
	}
	if c.Ease != startEase {
		t.Errorf("Ease = %v, want %v", c.Ease, startEase)
	}
}
