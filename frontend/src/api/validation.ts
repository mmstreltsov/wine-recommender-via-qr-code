import type { CartResponse, ConfigResponse, MenuItemFull, MenuItemSummary, MenuResponse, Recommendation, RecsResponse } from './types'

type ObjectValue = Record<string, unknown>

function object(value: unknown): ObjectValue {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid response')
  return value as ObjectValue
}

function text(value: unknown, required = false): string {
  if (typeof value !== 'string' || value.length > 10000 || (required && !value.trim())) throw new Error('Invalid response')
  return value
}

export function integer(value: unknown, min = 0, max = Number.MAX_SAFE_INTEGER): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < min || value > max) throw new Error('Invalid number')
  return value
}

function array(value: unknown): unknown[] {
  if (!Array.isArray(value) || value.length > 2000) throw new Error('Invalid response')
  return value
}

export function safeLocalImage(value: string): string {
  if (!value.startsWith('/static/') || /[\\\s]/.test(value) || Array.from(value).some((char) => char.charCodeAt(0) < 32)) return ''
  try {
    const decoded = decodeURIComponent(value)
    if (/[\\\s?#]/.test(decoded) || decoded.split('/').some((part) => part === '.' || part === '..') || Array.from(decoded).some((char) => char.charCodeAt(0) < 32)) return ''
    const url = new URL(value, window.location.origin)
    return url.origin === window.location.origin && url.pathname.startsWith('/static/') && !url.search && !url.hash ? url.pathname : ''
  } catch {
    return ''
  }
}

export function safeRemoteImage(value: string): string {
  try {
    const url = new URL(value)
    if (url.protocol !== 'https:' || url.username || url.password || url.port || url.search || url.hash) return ''
    if (!['static.tildacdn.com', 'optim.tildacdn.com', 'static.tildacdn.net', 'optim.tildacdn.net'].includes(url.hostname)) return ''
    return url.href
  } catch {
    return ''
  }
}

function summary(value: unknown): MenuItemSummary {
  const item = object(value)
  return {
    id: integer(item.id, 1), category_id: integer(item.category_id, 1),
    name: text(item.name, true), price: integer(item.price),
    photo_url: safeRemoteImage(text(item.photo_url)),
    photo_fallback: safeLocalImage(text(item.photo_fallback)),
  }
}

function recommendation(value: unknown): Recommendation {
  const rec = object(value)
  if (!['expert', 'upsell', 'heuristic'].includes(String(rec.source))) throw new Error('Invalid response')
  return { item: summary(rec.item), reason_text: text(rec.reason_text, true), source: rec.source as Recommendation['source'] }
}

export function parseMenu(value: unknown): MenuResponse {
  return { categories: array(object(value).categories).map((entry) => {
    const category = object(entry)
    return { id: integer(category.id, 1), name: text(category.name, true), items: array(category.items).map(summary) }
  }).filter((category) => category.items.length > 0) }
}

export function parseItem(value: unknown): MenuItemFull {
  const item = object(value)
  return { ...summary(value), description: text(item.description), kbju: text(item.kbju), upsell_options: array(item.upsell_options).map((value) => {
    const option = object(value)
    return { name: text(option.name, true), add_price: integer(option.add_price) }
  }) }
}

export function parseRecs(value: unknown): RecsResponse {
  return { recs: array(object(value).recs).map(recommendation) }
}

export function parseCart(value: unknown): CartResponse {
  const cart = object(value)
  const lines = array(cart.lines).map((value) => {
    const line = object(value)
    if (typeof line.available !== 'boolean' || !['menu', 'rec_feed', 'rec_cart'].includes(String(line.added_from))) throw new Error('Invalid response')
    return { item: summary(line.item), qty: integer(line.qty, 1, 20), available: line.available, added_from: line.added_from as CartResponse['lines'][number]['added_from'] }
  })
  const total = integer(cart.total)
  if (total !== lines.reduce((sum, line) => sum + line.item.price * line.qty, 0)) throw new Error('Invalid response')
  return { lines, total, recommendation: cart.recommendation === null ? null : recommendation(cart.recommendation) }
}

export function parseConfig(value: unknown): ConfigResponse {
  const config = object(value)
  const restaurant = object(config.restaurant)
  return { restaurant: { name: text(restaurant.name, true), logo_url: safeLocalImage(text(restaurant.logo_url)) }, rec_block_title: text(config.rec_block_title, true) }
}
