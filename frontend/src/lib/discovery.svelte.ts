// Discovery-item preview cache + tracking helpers.
//
// The frozen API contract has no "get series by anilist id" endpoint — discovery
// items already carry full metadata (titles, cover, banner, format, status). When
// a user opens an untracked discovery card we stash the item here and route to
// /series/anilist/:id so SeriesDetail can render a preview with zero extra calls.

import type { DiscoveryItem } from '$lib/api'

const previews = new Map<number, DiscoveryItem>()

export function rememberPreview(item: DiscoveryItem) {
  previews.set(item.anilist_id, item)
}

export function getPreview(anilistId: number): DiscoveryItem | null {
  return previews.get(anilistId) ?? null
}

// Track which anilist ids have been tracked this session, so discovery cards can
// optimistically flip to a "tracked" state without a full /tracked refetch.
export const trackedAnilistIds = $state(new Set<number>())

export function markTracked(anilistId: number) {
  trackedAnilistIds.add(anilistId)
}

export function markUntracked(anilistId: number) {
  trackedAnilistIds.delete(anilistId)
}
