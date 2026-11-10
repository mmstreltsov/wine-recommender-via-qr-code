import { useEffect, useRef, type MouseEvent, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import type { AddSource } from '../api/types'

interface EntryContext {
  source: AddSource
  returnTo: string
  parentIndex: number | null
  parentKey?: string
  parentState?: PageState
}

interface ViewState {
  scrollY: number
  focusId: string
  category: number | 'all'
}

interface PageState {
  entry?: EntryContext
  view?: ViewState
}

const record = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value)
const only = (value: Record<string, unknown>, keys: string[]) => Object.keys(value).every((key) => keys.includes(key))
const safeId = (id: string) => /^[1-9]\d*$/.test(id) && Number.isSafeInteger(Number(id))
export const safeReturnPath = (path: unknown): path is string => typeof path === 'string' && (path === '/' || path === '/cart' || (path.startsWith('/dish/') && safeId(path.slice(6))))

export function readPageState(value: unknown): PageState {
  return parsePageState(value, 0) ?? {}
}

const maxParentDepth = 4

function parsePageState(value: unknown, depth: number): PageState | null {
  if (!record(value) || !only(value, ['entry', 'view'])) return null
  const { entry, view } = value
  if (entry !== undefined && (!record(entry) || !only(entry, ['source', 'returnTo', 'parentIndex', 'parentKey', ...(depth < maxParentDepth ? ['parentState'] : [])]) ||
    typeof entry.source !== 'string' || !['menu', 'rec_feed', 'rec_cart'].includes(entry.source) || !safeReturnPath(entry.returnTo) ||
    !(entry.parentIndex === null || (Number.isSafeInteger(entry.parentIndex) && Number(entry.parentIndex) >= 0)) ||
    (entry.parentKey !== undefined && (typeof entry.parentKey !== 'string' || !/^[a-zA-Z0-9]{1,32}$/.test(entry.parentKey))) ||
    (entry.source === 'menu' && entry.returnTo !== '/') || (entry.source === 'rec_cart' && entry.returnTo !== '/cart') ||
    (entry.source === 'rec_feed' && !entry.returnTo.startsWith('/dish/')) ||
    (entry.parentState !== undefined && parsePageState(entry.parentState, depth + 1) === null))) return null
  if (view !== undefined && (!record(view) || !only(view, ['scrollY', 'focusId', 'category']) ||
    typeof view.scrollY !== 'number' || !Number.isFinite(view.scrollY) || view.scrollY < 0 || view.scrollY > 10_000_000 ||
    typeof view.focusId !== 'string' || !(view.focusId === 'page-heading' || (/^(?:menu|rec)-(?:[1-9]\d*)$/.test(view.focusId) && safeId(view.focusId.split('-')[1]))) ||
    !(view.category === 'all' || (Number.isSafeInteger(view.category) && Number(view.category) > 0)))) return null
  return value as PageState
}

function snapshot(value: PageState, depth: number): PageState {
  const { entry, view } = value
  return {
    ...(view && { view }),
    ...(entry && { entry: {
      source: entry.source, returnTo: entry.returnTo, parentIndex: entry.parentIndex,
      ...(entry.parentKey && { parentKey: entry.parentKey }),
      ...(depth < maxParentDepth - 1 && entry.parentState && { parentState: snapshot(entry.parentState, depth + 1) }),
    } }),
  }
}

function historyIndex(): number | null {
  const value: unknown = window.history.state
  return record(value) && Number.isSafeInteger(value.idx) && Number(value.idx) >= 0 ? Number(value.idx) : null
}

function previousEntryMatches(entry: EntryContext): boolean {
  const navigation = (window as Window & { navigation?: { currentEntry?: { index: number }; entries: () => { url?: string; getState: () => unknown }[] } }).navigation
  if (!navigation || !entry.parentKey || entry.parentIndex === null || historyIndex() !== entry.parentIndex + 1 || !navigation.currentEntry || navigation.currentEntry.index < 1) return false
  try {
    const previous = navigation.entries()[navigation.currentEntry.index - 1]
    const url = new URL(previous?.url ?? '')
    const state = previous.getState()
    return record(state) && state.key === entry.parentKey && url.origin === window.location.origin && url.pathname === entry.returnTo
  } catch {
    return false
  }
}

export function DishLink({ itemId, source, className, children, category = 'all' }: {
  itemId: number
  source: AddSource
  className: string
  children: ReactNode
  category?: number | 'all'
}) {
  const location = useLocation()
  const navigate = useNavigate()
  const focusId = `${source === 'menu' ? 'menu' : 'rec'}-${itemId}`
  const to = `/dish/${itemId}`

  function open(event: MouseEvent<HTMLAnchorElement>) {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || !safeReturnPath(location.pathname)) return
    event.preventDefault()
    const parent = readPageState(location.state)
    const parentState: PageState = {
      ...snapshot(parent, 0),
      view: { scrollY: window.scrollY, focusId, category },
    }
    const entry: EntryContext = { source, returnTo: location.pathname, parentIndex: historyIndex(), ...(/^[a-zA-Z0-9]{1,32}$/.test(location.key) && { parentKey: location.key }), parentState: readPageState(parentState) }
    const state: PageState = { entry }
    void navigate(location.pathname + location.search, {
      replace: true,
      state: { ...parent, view: parentState.view },
    })
    void navigate(to, { state })
  }

  if (!Number.isSafeInteger(itemId) || itemId < 1) return <div className={className}>{children}</div>
  return <Link id={focusId} to={to} className={className} onClick={open}>{children}</Link>
}

export function DishBackLink() {
  const location = useLocation()
  const navigate = useNavigate()
  const { entry } = readPageState(location.state)
  const to = entry?.returnTo ?? '/'

  function back(event: MouseEvent<HTMLAnchorElement>) {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    event.preventDefault()
    if (entry && previousEntryMatches(entry)) {
      void navigate(-1)
    } else {
      void navigate(to, { replace: true, state: entry?.parentState })
    }
  }

  return <Link to={to} className="back-link" onClick={back}>{to === '/' ? '← В меню' : to === '/cart' ? '← В корзину' : '← Назад к блюду'}</Link>
}

export function usePageRestore(ready: boolean, category: number | 'all' = 'all') {
  const location = useLocation()
  const restored = useRef<string | null>(null)
  useEffect(() => {
    if (!ready || restored.current === location.key) return
    restored.current = location.key
    const { view } = readPageState(location.state)
    const opener = view && document.getElementById(view.focusId)
    const target = opener ||
      (view?.focusId.startsWith('rec-') && document.querySelector<HTMLElement>('.rec-feed__title')) ||
      document.querySelector<HTMLElement>('main h1, main h2')
    target?.focus({ preventScroll: true })
    window.scrollTo(0, view?.scrollY ?? 0)
    if (view && !opener) target?.scrollIntoView?.({ block: 'nearest' })
  }, [ready, location.key, location.state])

  useEffect(() => {
    if (!ready) return
    function save() {
      const history: unknown = window.history.state
      if (!record(history) || (history.key ?? 'default') !== location.key) return
      const page = readPageState(history.usr)
      const id = document.activeElement?.id ?? ''
      const view = { scrollY: window.scrollY, focusId: /^(?:menu|rec)-[1-9]\d*$/.test(id) ? id : 'page-heading', category }
      window.history.replaceState({ ...history, usr: { ...page, view } }, '')
    }
    window.addEventListener('scroll', save, { passive: true })
    window.addEventListener('pagehide', save)
    document.addEventListener('focusin', save)
    return () => {
      window.removeEventListener('scroll', save)
      window.removeEventListener('pagehide', save)
      document.removeEventListener('focusin', save)
    }
  }, [ready, location.key, category])
}
