/**
 * Toggles a `scrolled` class on `node` whenever any scrollable descendant is
 * scrolled past a small threshold, so page headers can fade in a blur + scrim
 * once content slides under them. Attach to a page's root element; the capture
 * listener catches scroll from any inner scroll container (scroll doesn't
 * bubble, but the capture phase still reaches ancestors).
 */
export function scrollScrim(node: HTMLElement) {
  const THRESHOLD = 8

  function update(target: EventTarget | null) {
    const el = target as HTMLElement | null
    if (!el || typeof el.scrollTop !== 'number') return
    node.classList.toggle('scrolled', el.scrollTop > THRESHOLD)
  }

  function onScroll(e: Event) {
    update(e.target)
  }

  node.addEventListener('scroll', onScroll, true)

  return {
    destroy() {
      node.removeEventListener('scroll', onScroll, true)
    },
  }
}
