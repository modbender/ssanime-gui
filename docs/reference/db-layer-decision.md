# DB layer decision — sqlc + goose + modernc.org/sqlite

**Date:** 2026-06-06 · **Status:** Recommended (pending confirm)

## Decision

- **Driver:** `modernc.org/sqlite` — pure-Go, **cgo-free** (non-negotiable: cgo is what made the
  Wails build un-cross-compilable). Not `mattn/go-sqlite3` (cgo). Not `glebarez/sqlite` — that's
  only a GORM-compat fork of modernc; pointless without GORM.
- **Query layer:** `sqlc` — generates type-safe Go from raw SQL. Queries checked against the real
  schema at build time; zero runtime reflection.
- **Migrations:** `goose` — plain SQL migration files. sqlc reads the same files as its schema
  source, so schema and queries can't drift.

## Why sqlc over GORM *for this project specifically*

Our profile: tiny stable schema (~5 tables), fixed correctness-critical queries (status state
machine + crash-recovery resets), long-lived personal tool, single cgo-free binary mandatory.

- sqlc's strengths land exactly here: compile-time-safe fixed queries, explicit SQL visible in
  diffs, no runtime cost, pure-Go driver.
- GORM's strengths (AutoMigrate, dynamic query building) are low-value here, and its costs are real
  for us: reflection/N+1 footguns, opaque debugging, and AutoMigrate silently mutating the schema —
  an unacceptable risk for a tool guarding an irreplaceable archive.
- sqlc's main weakness — dynamic queries — barely applies; we have almost none.

`ent` (schema-as-Go-code codegen) and `bob`/`jet` (codegen query builders) were considered.
`ent` is more machinery to maintain for 5 tables (against the minimize-maintenance goal); sqlc has
the larger ecosystem and the goose synergy. No option is clearly better for our profile.

## Evidence

- 2025/2026 comparisons converge: *"sqlc hits a sweet spot: manual control with high safety and no
  runtime cost"*; *"Invalid SQL fails during generation, not in production"*; GORM = *"easiest to
  just ship but more runtime overhead"* + N+1 risk.
- r/golang lived experience: *"SQLC is the best of both worlds: typed API on top of SQL
  flexibility/predictability"*; *"pgx pools, sqlc, and Goose Migrate"* is a repeated default stack;
  *"SQLC can understand goose migrations, they play very well together."*
- Honest counterpoint (valid, recorded): for a solo dev who hates SQL and iterates schema rapidly,
  an ORM lets you *"focus on the idea instead of fixing migration problems."* Doesn't outweigh the
  cgo-free + archive-safety + fixed-schema factors here.

## The refinement that matters MORE than the ORM choice — SQLite concurrency

Our design runs **multiple worker goroutines** (download workers + encode worker) all updating the
same `items` rows. SQLite is **single-writer**; naive concurrent writes → `database is locked`.
This is a design constraint regardless of sqlc/GORM. From r/golang practitioners (incl. ncruces, a
SQLite-driver maintainer):

- **WAL mode** — readers never block; only writes take the file lock. Mandatory.
- **Single write connection.** Use two `*sql.DB` handles on the same file:
  - write handle: `SetMaxOpenConns(1)` (serializes all writes — no lock contention)
  - read handle: larger pool, concurrent reads under WAL
  - sqlc is agnostic — pass whichever `DBTX` (handle/tx) per query; this is a wiring choice.
- **Immediate transactions** for any writing tx (`_txlock=immediate` on DSN) to avoid
  lock-upgrade deadlocks ("`database is locked`" mid-transaction).
- Pragmas (copy from Seanime `database/db/db.go`): `journal_mode=WAL`, `busy_timeout=5000`,
  `foreign_keys=ON`, `synchronous=NORMAL`.

Seanime uses `MaxOpenConns(3)` with GORM; our split read/write-pool model is stricter and correct
for heavy concurrent status updates.

## Config gotcha to avoid

In `sqlc.yaml` set `engine: sqlite` (not postgres) — a mismatched engine is a common silent
"migrations don't apply / wrong types generated" trap.
