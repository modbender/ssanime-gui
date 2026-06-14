package source

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// searchEpisodesWorkers bounds the episode×provider search fan-out so a long
// season across several providers doesn't fire dozens of concurrent provider
// calls at once.
const searchEpisodesWorkers = 8

// SearchEpisodes runs a per-episode source search across every registered
// provider and returns the raw candidate releases bucketed by episode number,
// plus a deduplicated warning per fully-failed provider.
//
// Per-episode is required for correctness: Hayase extensions resolve each
// episode's anidbEid from ani.zip keyed by the queried episode number, so a
// number-0 bulk query resolves nothing ("No anidbEid provided"). One cell of
// work is issued per (episode, provider); base carries the shared SmartSearch
// template (query/resolution/best-releases/exclusions) and each cell overrides
// EpisodeNumber=n and Media=media so the two callers (server available, poller
// auto-fetch) share one search path.
//
// Bucketing: a returned torrent goes under the queried n unless its parsed
// EpisodeNumber is a different valid number (a batch/multi hit), in which case
// it is bucketed under that number. Warnings: a provider whose every queried
// cell errored yields one "<id>: <sample>" warning; when every provider fully
// failed the warnings collapse to a single actionable message.
func SearchEpisodes(ctx context.Context, reg *Registry, media Media, episodes []int, base SmartSearchOptions) (map[int][]*AnimeTorrent, []string) {
	providers := reg.List()
	if len(providers) == 0 {
		return map[int][]*AnimeTorrent{}, nil
	}

	type job struct {
		num int
		pid string
	}
	jobs := make([]job, 0, len(episodes)*len(providers))
	for _, n := range episodes {
		for _, pid := range providers {
			jobs = append(jobs, job{num: n, pid: pid})
		}
	}
	if len(jobs) == 0 {
		return map[int][]*AnimeTorrent{}, nil
	}

	type result struct {
		num      int
		pid      string
		torrents []*AnimeTorrent
		err      error
	}
	jobCh := make(chan job)
	resCh := make(chan result, len(jobs))

	workers := searchEpisodesWorkers
	if workers > len(jobs) {
		workers = len(jobs)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				if ctx.Err() != nil {
					resCh <- result{num: j.num, pid: j.pid, err: ctx.Err()}
					continue
				}
				p, ok := reg.Get(j.pid)
				if !ok {
					continue
				}
				opts := base
				opts.Media = media
				opts.EpisodeNumber = j.num
				torrents, err := p.SmartSearch(ctx, opts)
				if err != nil {
					resCh <- result{num: j.num, pid: j.pid, err: err}
					continue
				}
				resCh <- result{num: j.num, pid: j.pid, torrents: torrents}
			}
		}()
	}
	go func() {
		defer close(jobCh)
		for _, j := range jobs {
			select {
			case <-ctx.Done():
				return
			case jobCh <- j:
			}
		}
	}()
	go func() { wg.Wait(); close(resCh) }()

	candidates := map[int][]*AnimeTorrent{}
	// provStat records, per provider, whether every queried cell errored and one
	// sample error so a provider that rejects the same way for every episode
	// collapses to a single warning.
	type provStat struct {
		total  int
		failed int
		sample string
	}
	stats := map[string]*provStat{}
	for _, pid := range providers {
		stats[pid] = &provStat{}
	}

	for res := range resCh {
		if st := stats[res.pid]; st != nil {
			st.total++
			if res.err != nil {
				st.failed++
				if st.sample == "" {
					st.sample = res.err.Error()
				}
			}
		}
		if res.err != nil {
			continue
		}
		for _, t := range res.torrents {
			if t == nil {
				continue
			}
			// A valid different parsed episode (a batch/multi hit) buckets to its
			// own number; an unknown/<=0 parse buckets to the queried number.
			bucket := res.num
			if t.EpisodeNumber > 0 {
				bucket = t.EpisodeNumber
			}
			candidates[bucket] = append(candidates[bucket], t)
		}
	}

	var warnings []string
	fullyFailed := 0
	for _, pid := range providers {
		st := stats[pid]
		if st == nil || st.total == 0 {
			continue
		}
		if st.failed == st.total {
			fullyFailed++
			warnings = append(warnings, fmt.Sprintf("%s: %s", pid, st.sample))
		}
	}
	sort.Strings(warnings)

	// When every installed source failed for every episode, collapse the
	// per-provider noise into a single actionable message pointing at Extensions.
	if fullyFailed == len(providers) {
		warnings = []string{"All installed sources are unreachable — check Extensions."}
	}
	return candidates, warnings
}
