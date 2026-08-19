package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
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

// shortRef is the UID prefix nc undo shows and matches against — short
// enough to type, long enough that a collision across a handful of recent
// reviews is not a practical concern.
func shortRef(uid string) string {
	if len(uid) > 8 {
		return uid[:8]
	}
	return uid
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

	case "undo":
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to load config:", err)
			os.Exit(1)
		}

		// A ref is a positional arg, so it has to come before any flags —
		// same convention as log's <id> <grade>. Its absence (or a leading
		// "-") means "no ref given, just list recent entries".
		rest := os.Args[2:]
		var ref string
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
			ref = rest[0]
			rest = rest[1:]
		}

		fs := flag.NewFlagSet("undo", flag.ExitOnError)
		noSync := fs.Bool("no-sync", false, "skip git sync")
		fs.Parse(rest)

		syncPull(*noSync)

		events, err := store.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to load review history:", err)
			os.Exit(1)
		}
		active := store.ActiveEvents(events)

		if ref == "" {
			if len(active) == 0 {
				fmt.Println("no reviews logged yet")
				return
			}
			start := len(active) - 5
			if start < 0 {
				start = 0
			}
			recent := active[start:]
			for i := len(recent) - 1; i >= 0; i-- {
				e := recent[i]
				fmt.Printf("%s  %-6s %-40s %s\n", shortRef(e.UID), e.Grade, e.ID, e.At.Local().Format("2006-01-02 15:04"))
			}
			fmt.Println("run 'nc undo <ref>' to undo one of these")
			return
		}

		var target *store.Event
		for i := range active {
			if strings.HasPrefix(active[i].UID, ref) {
				if target != nil {
					fmt.Fprintf(os.Stderr, "%q matches more than one recent review — use a longer ref\n", ref)
					os.Exit(1)
				}
				target = &active[i]
			}
		}
		if target == nil {
			fmt.Fprintf(os.Stderr, "no recent review matches %q — run 'nc undo' to see recent entries\n", ref)
			os.Exit(1)
		}

		now := time.Now()
		voidEvent, err := store.Void(now, *target, cfg.Device)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to record undo:", err)
			os.Exit(1)
		}

		syncPush(*noSync, fmt.Sprintf("undo %s: %s", target.ID, target.Grade))

		card, ok := store.Fold(append(events, voidEvent))[target.ID]
		if !ok {
			fmt.Printf("undone: %s %s (logged %s) — %s has no other reviews, back to new\n",
				target.ID, target.Grade, target.At.Local().Format("2006-01-02 15:04"), target.ID)
		} else {
			fmt.Printf("undone: %s %s (logged %s) — %s now due %s (interval %dd, ease %.2f)\n",
				target.ID, target.Grade, target.At.Local().Format("2006-01-02 15:04"),
				target.ID, card.Due.Format("2006-01-02"), card.Interval, card.Ease)
		}

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

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		problems := catalog.Load()
		lastTopic := ""
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

			// Flushing at each topic change resets tabwriter's column
			// widths per section, so one long id/title elsewhere in the
			// catalog doesn't blow out padding for the whole list.
			if p.Topic != lastTopic {
				w.Flush()
				lastTopic = p.Topic
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Title, p.Topic)
		}
		w.Flush()

	case "help":
		printHelp(os.Stdout)

	default:
		fmt.Fprintln(os.Stderr, "Command not recognized. Run 'nc help' for usage.")
		os.Exit(1)
	}
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `nc - spaced repetition scheduler for the NeetCode 150

Usage: nc <command> [flags]

Commands:
  due  [--limit N]              List problems due for review today, most overdue first.
                                 --limit caps how many are shown (default from config).

  log  <id> <grade> [--mins N]  Record a review for a problem and reschedule it.
                                 <id> is a catalog id (see 'nc list'); <grade> is one of:
                                   study  - watched/read the solution (new or learning cards only)
                                   again  - got it wrong, needs to be seen again soon
                                   hard   - solved, but it was a struggle
                                   good   - solved comfortably
                                   easy   - solved with no hesitation
                                 --mins records time spent in minutes (default 25).

  undo [ref]                    Undo a mistaken log entry. With no ref, shows the last 5
                                 logged reviews with a short ref for each; run it again with
                                 one of those refs to undo it. Safe to run any time after —
                                 the log is append-only, so this adds an offsetting entry
                                 rather than deleting anything.

  list [--topic X] [--state Y]  Browse the full catalog. Filters combine (both must match).
                                 --topic filters by topic, e.g. "Arrays & Hashing".
                                 --state filters by card state: new, learning, or review.

  help                          Show this message.

All commands except help also accept --no-sync, which skips pulling/pushing the
review log's git repo for that invocation.

Data lives in ~/.config/neetcode-srs/config.json (settings) and the git repo
cloned to ~/.local/share/neetcode-srs/ (review history). See README.md and
DESIGN.md for details.
`)
}
