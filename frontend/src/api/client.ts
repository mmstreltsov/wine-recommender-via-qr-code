import type { AddSource, CartResponse, ConfigResponse } from './types'
import { integer, parseCart, parseConfig, parseItem, parseMenu, parseRecs } from './validation'

const API_BASE = '/api/v1'
const GENERIC_ERROR = 'Что-то пошло не так, попробуйте ещё раз'

export const DEFAULT_CONFIG: ConfigResponse = {
  restaurant: { name: 'Онлайн-меню', logo_url: '' },
  rec_block_title: 'Хорошо сочетается с',
}

export class ApiError extends Error {
  readonly status: number
  constructor(status: number) {
    super(GENERIC_ERROR)
    this.status = status
  }
}

async function request<T>(path: string, parse: (value: unknown) => T, init?: RequestInit): Promise<T> {
  try {
    const response = await fetch(`${API_BASE}${path}`, { credentials: 'same-origin', ...init })
    if (!response.ok) throw new ApiError(response.status)
    return parse(await response.json())
  } catch (error) {
    throw error instanceof ApiError ? error : new ApiError(0)
  }
}

function startTableParam(): string {
  const raw = new URLSearchParams(window.location.search).get('table')
  if (raw && /^\d{1,3}$/.test(raw) && Number(raw) >= 1 && Number(raw) <= 999) return `?table=${Number(raw)}`
  return ''
}

let bootPromise: Promise<void> | null = null
let started = false
const startListeners = new Set<() => void>()
let configCache = DEFAULT_CONFIG

export function isStarted(): boolean {
  return started
}

export function onStarted(listener: () => void): () => void {
  startListeners.add(listener)
  if (started) listener()
  return () => { startListeners.delete(listener) }
}

export function bootstrap(): Promise<void> {
  if (!bootPromise) {
    bootPromise = request(`/menu${startTableParam()}`, parseMenu).then(() => {
      started = true
      for (const listener of startListeners) listener()
    }).catch((error: unknown) => {
      bootPromise = null
      throw error
    })
  }
  return bootPromise
}

export const ensureStarted = bootstrap

export function getCachedConfig(): ConfigResponse {
  return configCache
}

export async function getMenu() {
  await ensureStarted()
  return request('/menu', parseMenu)
}

export async function getConfig() {
  await ensureStarted()
  configCache = await request('/config', parseConfig)
  return configCache
}

export async function getItem(id: number) {
  integer(id, 1)
  await ensureStarted()
  const item = await request(`/menu/items/${id}`, parseItem)
  if (item.id !== id) throw new ApiError(0)
  return item
}

export async function getRecommendations(id: number) {
  integer(id, 1)
  await ensureStarted()
  return request(`/menu/items/${id}/recommendations`, parseRecs)
}

type CartListener = (cart: CartResponse) => void
const cartListeners = new Set<CartListener>()
let lastCart: CartResponse | null = null
let cartQueue = Promise.resolve()

export function onCartUpdate(cb: CartListener): () => void {
  cartListeners.add(cb)
  return () => { cartListeners.delete(cb) }
}

export function getLastCart(): CartResponse | null {
  return lastCart
}

function cartRequest(path: string, init?: RequestInit): Promise<CartResponse> {
  const result = cartQueue.then(async () => {
    await ensureStarted()
    const cart = await request(path, parseCart, init)
    lastCart = cart
    for (const listener of cartListeners) listener(cart)
    return cart
  })
  cartQueue = result.then(() => {}, () => {})
  return result
}

export function getCart(): Promise<CartResponse> {
  return cartRequest('/cart')
}

export function addItem(item_id: number, source: AddSource): Promise<CartResponse> {
  integer(item_id, 1)
  if (!['menu', 'rec_feed', 'rec_cart'].includes(source)) return Promise.reject(new ApiError(0))
  return cartRequest('/cart/items', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ item_id, source }),
  })
}

export function setItemQty(item_id: number, qty: number): Promise<CartResponse> {
  integer(item_id, 1)
  integer(qty, 1, 20)
  return cartRequest(`/cart/items/${item_id}`, {
    method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ qty }),
  })
}

export function removeCartItem(item_id: number): Promise<CartResponse> {
  integer(item_id, 1)
  return cartRequest(`/cart/items/${item_id}`, { method: 'DELETE' })
}
