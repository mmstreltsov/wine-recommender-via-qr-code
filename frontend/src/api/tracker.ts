import { isStarted, onStarted } from './client'

export type ClientEventName =
  | 'page_open'
  | 'menu_view'
  | 'category_select'
  | 'dish_card_open'
  | 'rec_feed_visible'
  | 'rec_impression'
  | 'cart_open'
  | 'pay_click'
  | 'stub_view'

type Payload = Record<string, string | number | boolean>

const EVENTS_URL = '/api/v1/events'

let unloading = false
const queue: string[] = []

function flush(): void {
  if (!isStarted()) return
  for (const body of queue.splice(0)) void send(body)
}

onStarted(flush)

if (typeof window !== 'undefined') {
  window.addEventListener('pagehide', () => {
    unloading = true
    flush()
  })
  window.addEventListener('pageshow', () => { unloading = false })
}

async function send(body: string): Promise<void> {
  if (typeof document !== 'undefined' && (unloading || document.visibilityState === 'hidden')) {
    if (beacon(body)) return
  }
  try {
    const res = await fetch(EVENTS_URL, {
      method: 'POST',
      credentials: 'same-origin',
      keepalive: true,
      headers: { 'Content-Type': 'application/json' },
      body,
    })
    if (res.status === 429) return
  } catch {
    return
  }
}

function beacon(body: string): boolean {
  try {
    return navigator.sendBeacon?.(EVENTS_URL, new Blob([body], { type: 'application/json' })) ?? false
  } catch {
    return false
  }
}

export function track(name: ClientEventName, payload: Payload = {}): void {
  if (queue.length < 200) queue.push(JSON.stringify({ name, payload }))
  flush()
}

let pageOpened = false
export function trackPageOpen(): void {
  if (pageOpened) return
  pageOpened = true
  track('page_open')
}

export function trackMenuViewOnce(): void {
  track('menu_view')
}

export function trackCategorySelect(category_id: number): void {
  track('category_select', { category_id })
}

export function trackDishCardOpen(item_id: number): void {
  track('dish_card_open', { item_id, source: 'menu' })
}

const seenFeeds = new Set<number>()
export function trackRecFeedVisible(scope: number, dish_id: number, recs_count: number): void {
  if (seenFeeds.has(scope)) return
  seenFeeds.add(scope)
  track('rec_feed_visible', { dish_id, recs_count })
}

export function trackCartOpen(lines_count: number, total: number): void {
  track('cart_open', { lines_count, total })
}

export function trackPayClick(total: number, lines_count: number): void {
  track('pay_click', { total, lines_count })
}

export function trackStubView(): void {
  track('stub_view')
}

let scopeCounter = 0
const seenImpressions = new Set<string>()

export function nextImpressionScope(): number {
  scopeCounter += 1
  return scopeCounter
}

export function trackRecImpression(
  scope: number,
  context: 'dish' | 'cart',
  rec_item_id: number,
  rec_source: string,
): void {
  const key = `${scope}:${context}:${rec_item_id}`
  if (seenImpressions.has(key)) return
  seenImpressions.add(key)
  track('rec_impression', { context, rec_item_id, rec_source })
}
