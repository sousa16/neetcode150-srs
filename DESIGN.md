# neetcode150-srs — design

A CLI (`nc`) that schedules spaced repetition over the NeetCode 150, synced across two machines.

## Core decision: state is derived, never stored

The only thing written to disk is an **append-only event log**. Scheduling state (due date,
ease, interval) is recomputed by replaying that log on every run.

```
reviews.jsonl  ──fold──▶  map[problemID]Card  ──filter──▶  what's due today
```

Why: two devices writing one mutable `state.json` produces merge conflicts git cannot resolve.
Two devices *appending* to a JSONL file merge cleanly with `merge=union`. Full review history
for stats comes free.

## Storage

Two files, deliberately in different places.

**`problems.json`** — the static catalog of the 150 (id, title, url, topic, difficulty,
priority). Compiled into the binary with `go:embed`. Not user data, never changes, so it does
not belong in the data repo. `id` is the LeetCode slug; all 150 are unique, so it is the
primary key joining the catalog to the event log.

`priority` is `core` (129 problems) or `secondary` (21). Secondary is exactly three topics —
Advanced Graphs, Math & Geometry, Bit Manipulation — judged lower-yield for interviews. It
does **not** feed the SM-2 math; it only breaks ties when the daily cap forces a cut.

**`reviews.jsonl`** — the event log. Lives in a separate private repo
`neetcode150-srs-data`, cloned to `~/.local/share/neetcode-srs/`.

One event per line:

```json
{"uid":"01J...","id":"two-sum","at":"2026-08-18T14:32:11Z","grade":"good","mins":18,"device":"work"}
```

- `uid` — random, makes replay idempotent if a push retries and duplicates a line
- `at` — RFC3339, always **UTC**
- `grade` — `study | again | hard | good | easy`
- `mins` — optional, not used by the scheduler; for later analysis

`.gitattributes` in the data repo:

```
reviews.jsonl merge=union
```

## Scheduling: SM-2 with a learning phase

Three states: `new` → `learning` → `review`.

`study` is its own grade, for the first pass where you watch the O'Reilly solution rather than
solving it. That is not evidence about recall, so it must not feed the SM-2 math — it only
moves you into `learning` with a 1-day interval and an unaided re-attempt scheduled.

| state | grade | result |
|---|---|---|
| new | study | → learning, interval 1 |
| new | again/hard/good/easy | → review, interval 1 / 2 / 3 / 5 |
| learning | study | stay learning, interval 1 |
| learning | again | stay learning, interval 1 |
| learning | hard/good/easy | → review, interval 2 / 3 / 5 |
| review | again | → learning, interval 1, ease −0.20, lapses++ |
| review | hard | interval × 1.2, ease −0.15 |
| review | good | interval × ease |
| review | easy | interval × ease × 1.3, ease +0.15 |

- ease starts at 2.5, clamped to `[1.3, 2.7]`
- interval clamped to `[1, 120]` days — interview prep has a horizon, unlike vocabulary
- `due = date(at) + interval days`

### Day boundaries

Timestamps are stored UTC, but "is this due today" is a question about your **local calendar
day**, with a configurable start hour (default 04:00) so a 1am session counts as the previous
day. Comparing raw `time.Time` values instead of local dates is the classic bug here.

## Daily load

A flashcard review is 5 seconds; re-solving Word Ladder II is 40 minutes. So `due` caps output
at `daily_limit` (default 5). Anything over the cap stays due and reappears tomorrow — nothing
is lost, the list is just truncated.

Sort order, applied in sequence:

1. **most overdue first** — days between `due` and today
2. **`core` before `secondary`** — when the cap cuts the list, high-yield topics survive
3. **lowest ease first** — the problems you find hardest

Only the truncation is affected. A secondary problem that is genuinely due never disappears;
it just loses the race on a crowded day.

## Commands (v1)

```
nc due  [--limit N]              what to solve today
nc log  <id> <grade> [--mins N]  record a review
nc list [--topic X] [--state Y]  browse the catalog with current state
```

Global: `--no-sync` to skip git.

## Sync

Wraps git via `os/exec`:

- before a read: `git pull --rebase`
- after a write: `git add -A && git commit -m "..." && git push`

Failures are non-fatal — the local append already succeeded, so warn and continue. Being
offline must never block logging a review.

## Layout

```
cmd/nc/main.go          subcommand dispatch (stdlib flag, no cobra)
internal/catalog/       problems.json + go:embed
internal/srs/           Card, State, Grade, Schedule() — PURE, no I/O
internal/store/         JSONL append/load, fold to cards, git sync
internal/config/        ~/.config/neetcode-srs/config.json
```

`internal/srs` takes `now time.Time` as a parameter and never calls `time.Now()` or touches the
filesystem. That is what makes the scheduler table-testable, and it is where the tests go.

v1 is **stdlib only** — no external dependencies.

## Build order

1. `go mod init` + subcommand skeleton
2. `internal/catalog` + `nc list`
3. `internal/srs` + table-driven tests (no I/O yet)
4. `internal/store` + `nc due` / `nc log`
5. git sync
6. config file
