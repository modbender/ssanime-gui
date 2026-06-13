# Roadmap

Deferred work and larger initiatives that are intentionally out of the current
change set. Each entry records the motivation, the concrete plan, and the trigger
for picking it up — so the context isn't lost between sessions.

## Smart poller (polling scalability)

**Status:** deferred. The current poller is a single global scheduler that is
correct and light on CPU, but naive at high subscription counts. Revisit when a
user actually tracks many series (target: smooth at 200+).

### Current design (as of 2026-06-13)

- **One global poller**, single goroutine (`internal/poller/poller.go`). Not
  per-series, not layered.
- **Scheduler tick:** 60s (`defaultInterval`). Each wake runs one
  `ListFeedsDueForPoll` query and acts on the result.
- **Per-feed interval:** 3600s / 1h (`feeds.interval_seconds`). A feed is due when
  `last_checked_at + interval_seconds <= now` (or `last_checked_at IS NULL`).
- **Sequential processing:** feeds are polled one at a time (`for _, feed := range
  feeds`), no fan-out. Gentle on sources, light on CPU.
- **Gate** (`db/queries/feeds.sql` `ListFeedsDueForPoll`): `feed.enabled = 1 AND
  series.subscribed = 1 AND series.watch_status = 'watching' AND airing_status NOT
  IN (CANCELLED, NOT_YET_RELEASED) AND interval elapsed`.

### Problems at scale (200+ subscribed series)

Not CPU — the machine is fine. The real issues:

1. **Thundering herd on first boot.** `last_checked_at IS NULL` counts as due, so a
   fresh start (or a bulk-subscribe) makes *every* feed due at once. The first pass
   fires N source requests back-to-back.
2. **Source-side rate-limiting / bans.** N rapid sequential requests to one source
   (nyaa, etc.) is the classic pattern that gets an IP throttled or temporarily
   banned. `offset_seconds` defaults to `0`, so there is no jitter; feeds added
   together re-cluster their due-times.
3. **Head-of-line blocking.** A slow/hanging source stalls the rest of that pass
   until its HTTP timeout fires.

### Planned mitigations (cheap, no architecture change)

- **Per-pass cap** — poll at most N feeds per 60s tick (e.g. 20). 200 due then drains
  over ~10 min instead of one burst, and stays naturally rate-limited.
- **Jitter on subscribe** — randomize `offset_seconds` / initial `last_checked_at` so
  due-times disperse instead of clustering.
- **Polite per-request spacing + a sane per-feed HTTP timeout** so one hung source
  can't stall a pass.

### Larger "smart poller" ideas (further out)

- Adaptive intervals: poll actively-airing series more often, finished/slow ones
  rarely; back off on repeated empty results.
- Per-source concurrency + rate-limit budgets (group feeds by provider, respect each
  source's politeness window independently).
- Priority queue keyed on next-due rather than re-scanning all feeds each tick.
- Surface poll health (last success, error streak, next-due) per series in the UI.

**Trigger to pick up:** a real user tracking enough series that the boot burst or a
source throttle is observed, or before any "bulk import / OPML subscribe" feature
ships.
