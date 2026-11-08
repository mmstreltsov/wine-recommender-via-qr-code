import type { CartResponse, ConfigResponse, MenuItemFull, MenuResponse, Recommendation } from '../api/types'

export const summary = {
  id: 101, category_id: 1, name: 'Запечённые овощи', price: 650,
  photo_url: 'https://static.tildacdn.com/photos/vegetables.jpg',
  photo_fallback: '/static/restaurant/photos/vegetables.jpg',
}
export const dish: MenuItemFull = {
  ...summary,
  description: 'Овощи, зелень, 250 г', kbju: 'Калорийность: 150 ккал',
  upsell_options: [{ name: 'Свежая зелень', add_price: 100 }],
}
export const rec: Recommendation = {
  item: { ...summary, id: 202, category_id: 2, name: 'Тёплый хлеб', price: 200 },
  reason_text: 'Хрустящая корочка дополняет нежную текстуру запечённых овощей.', source: 'expert',
}
export const nextRec: Recommendation = { ...rec, item: { ...rec.item, id: 303, name: 'Домашний лимонад', price: 300 }, source: 'heuristic' }
export const config: ConfigResponse = { restaurant: { name: 'Ресторан', logo_url: '' }, rec_block_title: 'Хорошо сочетается с' }
export const menu: MenuResponse = { categories: [
  { id: 1, name: 'Основные блюда', items: [summary] },
  { id: 2, name: 'Хлеб', items: [rec.item] },
] }
export const emptyCart: CartResponse = { lines: [], total: 0, recommendation: null }
export const cart: CartResponse = { lines: [{ item: summary, qty: 1, available: true, added_from: 'menu' }], total: 650, recommendation: rec }
export function json(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
}
export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => { resolve = resolvePromise; reject = rejectPromise })
  return { promise, resolve, reject }
}
