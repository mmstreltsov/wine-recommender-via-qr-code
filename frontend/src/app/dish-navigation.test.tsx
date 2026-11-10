import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { DishBackLink, DishLink, readPageState, safeReturnPath, usePageRestore } from './dish-navigation'

function NavigationFixture() {
  const location = useLocation()
  usePageRestore(true)
  return <main><h1 tabIndex={-1}>Страница</h1><output data-testid="state">{JSON.stringify({ path: location.pathname, state: location.state })}</output>{location.pathname === '/dish/101' ? <DishLink itemId={202} source="rec_feed" className="link">Рекомендация</DishLink> : <DishBackLink />}</main>
}

function RestoreFixture({ opener }: { opener: boolean }) {
  usePageRestore(true)
  return <main><h1 tabIndex={-1}>Страница</h1><h2 tabIndex={-1} className="rec-feed__title">Рекомендации</h2>{opener && <a id="rec-42" href="/dish/42">Блюдо</a>}</main>
}

describe('RI-10 focus restoration', () => {
  it.each([true, false])('restores scroll and reveals fallback if opener is missing (opener exists: %s)', (opener) => {
    const scrollIntoView = vi.fn()
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    try {
      render(<MemoryRouter initialEntries={[{ pathname: '/cart', state: { view: { scrollY: 1200, focusId: 'rec-42', category: 'all' } } }]}><RestoreFixture opener={opener} /></MemoryRouter>)
      expect(scrollTo).toHaveBeenCalledWith(0, 1200)
      expect(document.activeElement).toBe(screen.getByText(opener ? 'Блюдо' : 'Рекомендации'))
      expect(scrollIntoView).toHaveBeenCalledTimes(opener ? 0 : 1)
    } finally {
      Element.prototype.scrollIntoView = original
      scrollTo.mockRestore()
    }
  })
})

describe('FR-05 FR-30 NFR-04 RI-10 RI-14 history state boundary', () => {
  it.each(['/', '/cart', '/dish/1', '/dish/9007199254740991'])('TestNFR04_internal_return_allowlist_%s', (path) => {
    expect(safeReturnPath(path)).toBe(true)
  })

  it.each(['//evil.test', 'https://evil.test', 'javascript:alert(1)', '/\\evil.test', '/dish/0', '/dish/-1', '/dish/01', '/dish/1e2', '/dish/9007199254740992', '/dish/%31', '/dish/1/..', '/dish/1?x=2', '/dish/1#x', '/cart/', '/pay-stub', '/?returnTo=/cart', '/dish/1\n', '', null, {}])('TestNFR04_reject_return_%j', (path) => {
    expect(safeReturnPath(path)).toBe(false)
    expect(readPageState({ entry: { source: 'rec_feed', returnTo: path, parentIndex: 0 } })).toEqual({})
  })

  it.each([
    null, [], 'menu', 3, { returnTo: '/cart' }, { extra: true },
    { entry: null }, { entry: [] }, { entry: { source: ['menu'], returnTo: '/', parentIndex: 0 } },
    { entry: { source: 'rec_cart', returnTo: '/', parentIndex: 0 } },
    { entry: { source: 'menu', returnTo: '/cart', parentIndex: 0 } },
    { entry: { source: 'rec_feed', returnTo: '/cart', parentIndex: 0 } },
    { entry: { source: 'menu', returnTo: '/', parentIndex: '0' } },
    { entry: { source: 'menu', returnTo: '/', parentIndex: Infinity } },
    { entry: { source: 'menu', returnTo: '/', parentIndex: 0, extra: 'no' } },
    { view: { scrollY: NaN, focusId: 'rec-1', category: 'all' } },
    { view: { scrollY: 0, focusId: '#selector<script>', category: 'all' } },
    { view: { scrollY: 0, focusId: 'rec-1', category: '1' } },
    { view: { scrollY: 0, focusId: 'rec-1', category: -1 } },
    { view: { scrollY: 0, focusId: 'rec-1', category: 1, extra: true } },
  ])('TestNFR04_all_unknown_state_fields_fail_closed_%j', (value) => {
    expect(readPageState(value)).toEqual({})
  })

  it('TestFR30_valid_entry_and_view_survive_history_serialization', () => {
    const value = { entry: { source: 'rec_feed', returnTo: '/dish/101', parentIndex: 2 }, view: { scrollY: 1234.5, focusId: 'rec-202', category: 'all' } }
    expect(readPageState(JSON.parse(JSON.stringify(value)))).toEqual(value)
  })

  it('TestFR30_RI10_reload_fallback_restores_parent_source_return_and_view', () => {
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    const originalScroll = window.scrollY
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 750 })
    try {
      render(<MemoryRouter initialEntries={[{ pathname: '/dish/101', state: { entry: { source: 'rec_cart', returnTo: '/cart', parentIndex: 0 } } }]}><NavigationFixture /></MemoryRouter>)
      fireEvent.click(screen.getByText('Рекомендация'))
      const saved = JSON.parse(screen.getByTestId('state').textContent!)
      expect(saved.state.entry.parentState).toEqual({ entry: { source: 'rec_cart', returnTo: '/cart', parentIndex: 0 }, view: { scrollY: 750, focusId: 'rec-202', category: 'all' } })
      cleanup()
      render(<MemoryRouter initialEntries={[JSON.parse(JSON.stringify({ pathname: saved.path, state: saved.state }))]}><NavigationFixture /></MemoryRouter>)
      fireEvent.click(screen.getByText('← Назад к блюду'))
      const restored = JSON.parse(screen.getByTestId('state').textContent!)
      expect(restored.path).toBe('/dish/101')
      expect(restored.state.entry.source).toBe('rec_cart')
      expect(restored.state.entry.returnTo).toBe('/cart')
      expect(scrollTo).toHaveBeenLastCalledWith(0, 750)
      expect(document.activeElement).toBe(screen.getByText('Рекомендация'))
    } finally {
      Object.defineProperty(window, 'scrollY', { configurable: true, value: originalScroll })
      scrollTo.mockRestore()
    }
  })

  it('TestNFR04_rejects_malicious_or_recursive_parent_snapshots', () => {
    const entry = { source: 'rec_feed', returnTo: '/dish/101', parentIndex: 1 }
    const recursive: Record<string, unknown> = { entry: { ...entry } }
    ;(recursive.entry as Record<string, unknown>).parentState = recursive
    for (const parentState of [recursive, { entry: { source: 'rec_cart', returnTo: '//evil.test', parentIndex: 0 } }, { view: { scrollY: 0, focusId: 'x'.repeat(10000), category: 'all' } }, { secret: 'not allowed' }]) {
      expect(readPageState({ entry: { ...entry, parentState } })).toEqual({})
    }
  })
})
