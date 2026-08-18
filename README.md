# neetcode150-srs

A CLI (`nc`) that schedules spaced repetition over the NeetCode 150, synced across machines via
a private git repo. See [DESIGN.md](DESIGN.md) for the full design (storage model, SM-2
scheduling rules, sync behavior).

## Build

```
go build -o nc ./cmd/nc
```

Requires Go 1.25+ and `git` on `PATH`. No external Go dependencies.

## Commands

```
nc due  [--limit N]              what to solve today
nc log  <id> <grade> [--mins N]  record a review (grade: study|again|hard|good|easy)
nc list [--topic X] [--state Y]  browse the catalog (state: new|learning|review)
nc help                          show usage for all commands
```

`due`, `log`, and `list` all accept `--no-sync` to skip git sync for that invocation. Run
`nc help` for a full explanation of each command's flags and grades.

## First run

On first use, `nc` creates `~/.config/neetcode-srs/config.json` with defaults (edit it to
change `start_hour`, `daily_limit`, `data_repo`, or `device`), and clones `data_repo` into
`~/.local/share/neetcode-srs/` to hold `reviews.jsonl`, the append-only review log. That repo
needs a `.gitattributes` with `reviews.jsonl merge=union` so concurrent reviews from two
machines merge cleanly.

## Test

```
go test ./...
```
