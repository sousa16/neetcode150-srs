package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/sousa16/neetcode150-srs/internal/catalog"
	"github.com/sousa16/neetcode150-srs/internal/config"
	"github.com/sousa16/neetcode150-srs/internal/srs"
	"github.com/sousa16/neetcode150-srs/internal/store"
)

type dueItem struct {
	problem     catalog.Problem
	card        srs.Card
	daysOverdue int
}

func localDay(t time.Time, startHour int) time.Time {
	local := t.Local()
	if local.Hour() < startHour {
		local = local.AddDate(0, 0, -1)
	}
	y, m, d := local.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// syncPull and syncPush wrap the store package's git sync so a failure
// warns and continues rather than aborting the command — per DESIGN.md,
// being offline must never block a read or a logged review.
func syncPull(noSync bool) {
	if noSync {
		return
	}
	if err := store.Pull(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: git sync (pull) failed:", err)
	}
}

func syncPush(noSync bool, message string) {
	if noSync {
		return
	}
	if err := store.Push(message); err != nil {
		fmt.Fprintln(os.Stderr, "warning: git sync (push) failed:", err)
	}
}

func main() {
	if len(os.Args) < 2 {
		return
	}

	switch os.Args[1] {
	case "due":
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to load config:", err)
			os.Exit(1)
		}

		fs := flag.NewFlagSet("due", flag.ExitOnError)
		limit := fs.Int("limit", cfg.DailyLimit, "max problems to show")
		noSync := fs.Bool("no-sync", false, "skip git sync")
		fs.Parse(os.Args[2:])
		if *limit <= 0 {
			fmt.Fprintln(os.Stderr, "Invalid limit")
			os.Exit(1)
		}

		syncPull(*noSync)

		events, err := store.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to load review history:", err)
			os.Exit(1)
		}
		cards := store.Fold(events)

		problems := catalog.Load()
		today := localDay(time.Now(), cfg.StartHour)
		priorityRank := map[string]int{"core": 0, "secondary": 1}

		var due []dueItem
		for _, p := range problems {
			card, ok := cards[p.ID]
			if !ok {
				continue // never reviewed: not on the schedule yet
			}

			dueDate := card.Due.Local()
			y, m, d := dueDate.Date()
			dueDay := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
			if dueDay.After(today) {
				continue
			}

			daysOverdue := int(today.Sub(dueDay).Hours() / 24)
			due = append(due, dueItem{problem: p, card: card, daysOverdue: daysOverdue})
		}

		sort.Slice(due, func(i, j int) bool {
			a, b := due[i], due[j]
			if a.daysOverdue != b.daysOverdue {
				return a.daysOverdue > b.daysOverdue
			}
			if priorityRank[a.problem.Priority] != priorityRank[b.problem.Priority] {
				return priorityRank[a.problem.Priority] < priorityRank[b.problem.Priority]
			}
			return a.card.Ease < b.card.Ease
		})

		if len(due) > *limit {
			due = due[:*limit]
		}

		for _, item := range due {
			fmt.Println(item.problem.ID, item.problem.Title, item.problem.Topic)
		}

	case "log":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: nc log <id> <grade> [--mins N]")
			os.Exit(1)
		}

		id := os.Args[2]
		problems := catalog.Load()
		validIDs := make(map[string]bool, len(problems))
		for _, p := range problems {
			validIDs[p.ID] = true
		}
		if !validIDs[id] {
			fmt.Fprintf(os.Stderr, "unknown problem id %q\n", id)
			os.Exit(1)
		}

		grade := os.Args[3]
		validGrades := map[string]bool{
			"study": true, "again": true, "hard": true, "good": true, "easy": true,
		}
		if !validGrades[grade] {
			fmt.Fprintf(os.Stderr, "invalid grade %q: must be one of study, again, hard, good, easy\n", grade)
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to load config:", err)
			os.Exit(1)
		}

		fs := flag.NewFlagSet("log", flag.ExitOnError)
		mins := fs.Int("mins", 25, "time the problem took")
		noSync := fs.Bool("no-sync", false, "skip git sync")
		fs.Parse(os.Args[4:])
		if *mins <= 0 {
			fmt.Fprintln(os.Stderr, "Invalid number of minutes")
			os.Exit(1)
		}

		now := time.Now()

		syncPull(*noSync)

		events, err := store.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to load review history:", err)
			os.Exit(1)
		}
		card, ok := store.Fold(events)[id]
		if !ok {
			card = srs.NewCard(id)
		}

		next, err := srs.Schedule(now, card, srs.Grade(grade))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if _, err := store.Append(now, id, srs.Grade(grade), *mins, cfg.Device); err != nil {
			fmt.Fprintln(os.Stderr, "failed to record review:", err)
			os.Exit(1)
		}

		syncPush(*noSync, fmt.Sprintf("log %s: %s", id, grade))

		fmt.Printf("logged %s as %s — next due %s (interval %dd, ease %.2f)\n",
			id, grade, next.Due.Format("2006-01-02"), next.Interval, next.Ease)

	case "list":
		fs := flag.NewFlagSet("list", flag.ExitOnError)
		topic := fs.String("topic", "", "filter by topic")
		state := fs.String("state", "", "filter by state (new, learning, review)")
		noSync := fs.Bool("no-sync", false, "skip git sync")
		fs.Parse(os.Args[2:])

		if *state != "" {
			validStates := map[string]bool{"new": true, "learning": true, "review": true}
			if !validStates[*state] {
				fmt.Fprintf(os.Stderr, "invalid state %q: must be one of new, learning, review\n", *state)
				os.Exit(1)
			}
		}

		syncPull(*noSync)

		events, err := store.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to load review history:", err)
			os.Exit(1)
		}
		cards := store.Fold(events)

		problems := catalog.Load()
		for _, p := range problems {
			if *topic != "" && p.Topic != *topic {
				continue
			}

			card, ok := cards[p.ID]
			if !ok {
				card = srs.NewCard(p.ID)
			}
			if *state != "" && string(card.State) != *state {
				continue
			}

			fmt.Println(p.ID, p.Title, p.Topic)
		}

	default:
		fmt.Fprintln(os.Stderr, "Command not recognized")
		os.Exit(1)
	}
}
