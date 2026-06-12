/// <reference types="bun-types" />
import { describe, expect, test } from 'bun:test'
import {
  encodeAvg,
  episodeOverall,
  episodeStage,
  isActive,
  overallPercentOf,
  type EpisodeProgressInput,
  type OutputLive,
} from './pipeline-math'

const close = (a: number, b: number, eps = 1e-9) => Math.abs(a - b) < eps

describe('encodeAvg', () => {
  test('no outputs → 0', () => {
    expect(encodeAvg([])).toBe(0)
  })

  test('single finished output → 100', () => {
    expect(encodeAvg([{ status: 'archived' }])).toBe(100)
    expect(encodeAvg([{ status: 'encoded' }])).toBe(100)
  })

  test('single queued output → 0', () => {
    expect(encodeAvg([{ status: 'queued' }])).toBe(0)
  })

  test('single encoding output → its live percent', () => {
    expect(encodeAvg([{ status: 'encoding', percent: 40 }])).toBe(40)
  })

  test('encoding output with no live percent → 0', () => {
    expect(encodeAvg([{ status: 'encoding' }])).toBe(0)
  })

  test('spec mix [archived, encoding 40, queued] → 46.666…', () => {
    const outs: OutputLive[] = [
      { status: 'archived' },
      { status: 'encoding', percent: 40 },
      { status: 'queued' },
    ]
    expect(close(encodeAvg(outs), (100 + 40 + 0) / 3)).toBe(true)
  })

  test('clamps out-of-range live percent', () => {
    expect(encodeAvg([{ status: 'encoding', percent: 150 }])).toBe(100)
    expect(encodeAvg([{ status: 'encoding', percent: -10 }])).toBe(0)
  })

  test('error / unknown outputs contribute 0', () => {
    expect(encodeAvg([{ status: 'error' }, { status: 'archived' }])).toBe(50)
  })
})

describe('episodeOverall — across every status', () => {
  const base: Omit<EpisodeProgressInput, 'status'> = { downloadPercent: null, outputs: [] }

  test('queued → 0', () => {
    expect(episodeOverall({ ...base, status: 'queued' })).toBe(0)
  })

  test('downloading → download.percent * 0.5', () => {
    expect(episodeOverall({ ...base, status: 'downloading', downloadPercent: 0 })).toBe(0)
    expect(episodeOverall({ ...base, status: 'downloading', downloadPercent: 50 })).toBe(25)
    expect(episodeOverall({ ...base, status: 'downloading', downloadPercent: 100 })).toBe(50)
  })

  test('downloading with no live percent → 0', () => {
    expect(episodeOverall({ ...base, status: 'downloading' })).toBe(0)
  })

  test('downloaded → 50', () => {
    expect(episodeOverall({ ...base, status: 'downloaded' })).toBe(50)
  })

  test('encoding → 50 + encodeAvg * 0.5 (spec mix → 73.33…)', () => {
    const outputs: OutputLive[] = [
      { status: 'archived' },
      { status: 'encoding', percent: 40 },
      { status: 'queued' },
    ]
    const v = episodeOverall({ ...base, status: 'encoding', outputs })
    expect(close(v, 50 + ((100 + 40 + 0) / 3) * 0.5)).toBe(true)
    // sanity: ≈ 73.333
    expect(v > 73.3 && v < 73.4).toBe(true)
  })

  test('encoding with all outputs queued → 50 (no advance yet)', () => {
    const outputs: OutputLive[] = [{ status: 'queued' }, { status: 'queued' }]
    expect(episodeOverall({ ...base, status: 'encoding', outputs })).toBe(50)
  })

  test('encoding with all outputs done → 100', () => {
    const outputs: OutputLive[] = [{ status: 'encoded' }, { status: 'archived' }]
    expect(episodeOverall({ ...base, status: 'encoding', outputs })).toBe(100)
  })

  test('encoded → ~100 (full bar, no tick)', () => {
    expect(episodeOverall({ ...base, status: 'encoded' })).toBe(100)
  })

  test('thumbnailing → ~100', () => {
    expect(episodeOverall({ ...base, status: 'thumbnailing' })).toBe(100)
  })

  test('archived → 100', () => {
    expect(episodeOverall({ ...base, status: 'archived' })).toBe(100)
  })

  test('error in encode half freezes at last encode %', () => {
    const outputs: OutputLive[] = [{ status: 'archived' }, { status: 'encoding', percent: 40 }, { status: 'queued' }]
    const v = episodeOverall({ ...base, status: 'error', outputs })
    expect(close(v, 50 + ((100 + 40 + 0) / 3) * 0.5)).toBe(true)
  })

  test('error in download half freezes at last download %', () => {
    const v = episodeOverall({ ...base, status: 'error', downloadPercent: 30 })
    expect(v).toBe(15)
  })

  test('unknown status → 0', () => {
    expect(episodeOverall({ ...base, status: 'whatever' })).toBe(0)
  })
})

describe('episodeStage', () => {
  test('maps statuses to coarse stages', () => {
    expect(episodeStage('queued')).toBe('queued')
    expect(episodeStage('downloading')).toBe('downloading')
    expect(episodeStage('downloaded')).toBe('encoding')
    expect(episodeStage('encoding')).toBe('encoding')
    expect(episodeStage('encoded')).toBe('encoding')
    expect(episodeStage('thumbnailing')).toBe('encoding')
    expect(episodeStage('archived')).toBe('done')
    expect(episodeStage('error')).toBe('error')
    expect(episodeStage('mystery')).toBe('queued')
  })
})

describe('isActive', () => {
  test('only downloading + encoding are active', () => {
    expect(isActive('downloading')).toBe(true)
    expect(isActive('encoding')).toBe(true)
    expect(isActive('queued')).toBe(false)
    expect(isActive('downloaded')).toBe(false)
    expect(isActive('encoded')).toBe(false)
    expect(isActive('archived')).toBe(false)
    expect(isActive('error')).toBe(false)
  })
})

describe('overallPercentOf', () => {
  test('null when no active inputs', () => {
    expect(overallPercentOf([])).toBeNull()
  })

  test('mean episodeOverall across inputs', () => {
    const inputs: EpisodeProgressInput[] = [
      { status: 'downloading', downloadPercent: 100, outputs: [] }, // 50
      { status: 'downloaded', downloadPercent: null, outputs: [] }, // 50
    ]
    expect(overallPercentOf(inputs)).toBe(50)
  })

  test('mixes download and encode halves', () => {
    const inputs: EpisodeProgressInput[] = [
      { status: 'downloading', downloadPercent: 40, outputs: [] }, // 20
      { status: 'encoding', downloadPercent: null, outputs: [{ status: 'archived' }] }, // 100
    ]
    expect(overallPercentOf(inputs)).toBe(60)
  })
})
