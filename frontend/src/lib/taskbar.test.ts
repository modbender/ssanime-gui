/// <reference types="bun-types" />
import { afterEach, describe, expect, test } from 'bun:test'
import { isTauri } from './taskbar'

describe('isTauri — sink selection', () => {
  const w = globalThis as unknown as { window?: unknown }
  const original = w.window

  afterEach(() => {
    w.window = original
  })

  test('false in a plain environment (no __TAURI_INTERNALS__)', () => {
    w.window = {}
    expect(isTauri()).toBe(false)
  })

  test('true when the Tauri internals marker is present', () => {
    w.window = { __TAURI_INTERNALS__: {} }
    expect(isTauri()).toBe(true)
  })

  test('false when window is undefined (SSR)', () => {
    w.window = undefined
    expect(isTauri()).toBe(false)
  })
})
