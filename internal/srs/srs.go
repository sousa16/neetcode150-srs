package srs

import (
	"errors"
	"math"
	"time"
)

type State string

const (
	StateNew      State = "new"
	StateLearning State = "learning"
	StateReview   State = "review"
)

type Grade string

const (
	GradeStudy Grade = "study"
	GradeAgain Grade = "again"
	GradeHard  Grade = "hard"
	GradeGood  Grade = "good"
	GradeEasy  Grade = "easy"
)

const (
	startEase = 2.5
	minEase   = 1.3
	maxEase   = 2.7

	minInterval = 1
	maxInterval = 120
)

type Card struct {
	ID       string
	State    State
	Interval int
	Ease     float64
	Due      time.Time
	Lapses   int
}

// ErrStudyNotAllowed is returned by Schedule when a card in the review state
// is graded study — DESIGN.md's table only defines study for new/learning;
// by the time a card reaches review, "watching the solution" is no longer a
// meaningful grade for it.
var ErrStudyNotAllowed = errors.New("study is only valid for new or learning cards")

func NewCard(id string) Card {
	return Card{ID: id, State: StateNew, Ease: startEase}
}

func Schedule(now time.Time, card Card, grade Grade) (Card, error) {
	if card.State == StateReview && grade == GradeStudy {
		return card, ErrStudyNotAllowed
	}

	next := card

	switch card.State {
	case StateNew:
		next.State, next.Interval = scheduleNew(grade)

	case StateLearning:
		next.State, next.Interval = scheduleLearning(grade)

	case StateReview:
		next.State = StateReview
		switch grade {
		case GradeAgain:
			next.State = StateLearning
			next.Interval = 1
			next.Ease = clampEase(card.Ease - 0.20)
			next.Lapses = card.Lapses + 1
		case GradeHard:
			next.Interval = round(float64(card.Interval) * 1.2)
			next.Ease = clampEase(card.Ease - 0.15)
		case GradeGood:
			next.Interval = round(float64(card.Interval) * card.Ease)
		case GradeEasy:
			next.Interval = round(float64(card.Interval) * card.Ease * 1.3)
			next.Ease = clampEase(card.Ease + 0.15)
		}
	}

	next.Interval = clampInterval(next.Interval)
	next.Due = dateOnly(now).AddDate(0, 0, next.Interval)

	return next, nil
}

// A study/again grade on a brand-new card still lands in review with a
// 1-day interval per DESIGN.md's table — only study on a new card enters
// the learning phase.
func scheduleNew(grade Grade) (State, int) {
	if grade == GradeStudy {
		return StateLearning, 1
	}
	return StateReview, intervalForFirstPass(grade)
}

func scheduleLearning(grade Grade) (State, int) {
	if grade == GradeStudy || grade == GradeAgain {
		return StateLearning, 1
	}
	return StateReview, intervalForFirstPass(grade)
}

func intervalForFirstPass(grade Grade) int {
	switch grade {
	case GradeAgain:
		return 1
	case GradeHard:
		return 2
	case GradeGood:
		return 3
	case GradeEasy:
		return 5
	}
	return 1
}

func clampEase(e float64) float64 {
	switch {
	case e < minEase:
		return minEase
	case e > maxEase:
		return maxEase
	default:
		return e
	}
}

func clampInterval(i int) int {
	switch {
	case i < minInterval:
		return minInterval
	case i > maxInterval:
		return maxInterval
	default:
		return i
	}
}

func round(f float64) int {
	return int(math.Round(f))
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
