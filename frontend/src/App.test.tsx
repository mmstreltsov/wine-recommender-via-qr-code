import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import App from './App'
import { trackPageOpen } from './api/tracker'
import type { CartResponse } from './api/types'
import { config, dish, emptyCart, json, menu, nextRec, rec, summary } from './test/fixtures'
import { TestIntersectionObserver } from './vitest.setup'

it('TestFR01_FR20_UI04_AN02_guest_path_startup_retry_impressions_cart_and_stub_end_to_end', async () => {
  window.history.replaceState({}, '', '/?table=12')
  let startupFailed = false
  const serverCart: CartResponse = structuredClone(emptyCart)
  const events: { name: string; payload: Record<string, unknown> }[] = []
  const mutations: { item_id: number; source: string }[] = []
  const beacon = vi.fn(() => true)
  Object.defineProperty(navigator, 'sendBeacon', { configurable: true, value: beacon })
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (path === '/api/v1/menu?table=12') {
      if (!startupFailed) { startupFailed = true; return json({ error: { message: 'private implementation detail' } }, 500) }
      return json(menu)
    }
    if (path === '/api/v1/events') {
      events.push(JSON.parse(String(init?.body)))
      return new Response(null, { status: 204 })
    }
    if (path === '/api/v1/config') return json(config)
    if (path === '/api/v1/menu') return json(menu)
    if (path === '/api/v1/cart') return json(serverCart)
    if (path === '/api/v1/menu/items/101') return json(dish)
    if (path === '/api/v1/menu/items/101/recommendations') return json({ recs: serverCart.lines.some((line) => line.item.id === rec.item.id) ? [] : [rec] })
    if (path === '/api/v1/cart/items' && init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as { item_id: number; source: 'menu' | 'rec_feed' | 'rec_cart' }
      mutations.push(body)
      const item = [summary, rec.item, nextRec.item].find((item) => item.id === body.item_id)!
      serverCart.lines.push({ item, qty: 1, available: true, added_from: body.source })
      if (body.item_id === nextRec.item.id) serverCart.lines[0].available = false
    } else if (path.startsWith('/api/v1/cart/items/') && init?.method === 'PATCH') {
      const id = Number(path.split('/').pop())
      serverCart.lines.find((line) => line.item.id === id)!.qty = JSON.parse(String(init.body)).qty
    } else if (path.startsWith('/api/v1/cart/items/') && init?.method === 'DELETE') {
      const id = Number(path.split('/').pop())
      serverCart.lines = serverCart.lines.filter((line) => line.item.id !== id)
    } else {
      throw new Error('Unexpected request')
    }
    serverCart.total = serverCart.lines.reduce((sum, line) => sum + line.item.price * line.qty, 0)
    serverCart.recommendation = serverCart.lines.length === 0 ? null : serverCart.lines.some((line) => line.item.id === rec.item.id) ? serverCart.lines.some((line) => line.item.id === nextRec.item.id) ? null : nextRec : rec
    return json(serverCart, init?.method === 'POST' ? 201 : 200)
  })
  vi.stubGlobal('fetch', fetcher)
  trackPageOpen()
  render(<App />)
  expect(await screen.findByRole('alert')).toHaveTextContent('Что-то пошло не так, попробуйте ещё раз')
  expect(fetcher).toHaveBeenCalledTimes(1)
  window.dispatchEvent(new Event('pagehide'))
  expect(beacon).not.toHaveBeenCalled()
  expect(events).toEqual([])
  window.dispatchEvent(new Event('pageshow'))
  fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
  fireEvent.click(await screen.findByRole('link', { name: new RegExp(dish.name) }))
  await screen.findByRole('heading', { name: dish.name })
  const firstHeading = screen.getByRole('heading', { name: config.rec_block_title })
  const firstCard = screen.getByText(rec.item.name).closest('.rec-card')!
  act(() => { TestIntersectionObserver.show(firstHeading, 0.49); TestIntersectionObserver.show(firstCard, 0.49) })
  expect(events.filter((event) => event.name === 'rec_impression')).toHaveLength(0)
  act(() => { TestIntersectionObserver.show(firstHeading, 0.5); TestIntersectionObserver.show(firstCard, 0.5); TestIntersectionObserver.show(firstCard, 1) })
  expect(events.filter((event) => event.name === 'rec_feed_visible')).toHaveLength(1)
  expect(events.filter((event) => event.name === 'rec_impression')).toHaveLength(1)
  fireEvent.click(screen.getByRole('link', { name: '← В меню' }))
  fireEvent.click(await screen.findByRole('link', { name: new RegExp(dish.name) }))
  await screen.findByRole('heading', { name: dish.name })
  act(() => {
    TestIntersectionObserver.show(screen.getByRole('heading', { name: config.rec_block_title }), 0.5)
    TestIntersectionObserver.show(screen.getByText(rec.item.name).closest('.rec-card')!, 0.5)
  })
  expect(events.filter((event) => event.name === 'rec_impression')).toHaveLength(2)
  expect(events.filter((event) => event.name === 'rec_feed_visible')).toHaveLength(2)
  fireEvent.click(screen.getByRole('button', { name: 'Добавить в корзину' }))
  await screen.findByText('✓ Добавлено')
  fireEvent.click(screen.getByRole('button', { name: `Добавить «${rec.item.name}» в корзину` }))
  await screen.findAllByText('✓ Добавлено')
  await waitFor(() => expect(screen.queryByRole('heading', { name: config.rec_block_title })).not.toBeInTheDocument(), { timeout: 3000 })
  fireEvent.click(screen.getByRole('link', { name: 'Корзина, позиций: 2' }))
  fireEvent.click(await screen.findByRole('button', { name: `Добавить «${nextRec.item.name}» в корзину` }))
  await screen.findByText('Нет в наличии')
  expect(screen.getByText('1 150 ₽')).toBeInTheDocument()
  expect(screen.getAllByRole('listitem')).toHaveLength(3)
  expect(screen.getByRole('link', { name: 'Корзина, позиций: 3' })).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Оплатить' }))
  expect(screen.getByRole('alert')).toHaveTextContent('Часть позиций больше нет в наличии')
  expect(events.some((event) => event.name === 'pay_click' || event.name === 'stub_view')).toBe(false)
  fireEvent.click(screen.getAllByRole('button', { name: 'Увеличить количество' })[0])
  await screen.findByText('1 800 ₽')
  fireEvent.click(screen.getByRole('button', { name: `Удалить «${dish.name}» из корзины` }))
  await waitFor(() => expect(screen.queryByText('Нет в наличии')).not.toBeInTheDocument())
  fireEvent.click(screen.getByRole('button', { name: 'Оплатить' }))
  await screen.findByText('Ваш выбор сформирован. Позовите официанта.')
  expect(window.location.pathname).toBe('/pay-stub')
  expect(events.filter((event) => event.name === 'page_open')).toHaveLength(1)
  expect(events.filter((event) => event.name === 'pay_click')).toEqual([{ name: 'pay_click', payload: { total: 500, lines_count: 2 } }])
  expect(events.filter((event) => event.name === 'stub_view')).toHaveLength(1)
  expect(events.some((event) => ['rec_add_click', 'menu_add_click', 'cart_item_remove', 'cart_qty_change'].includes(event.name))).toBe(false)
  expect(mutations).toEqual([{ item_id: 101, source: 'menu' }, { item_id: 202, source: 'rec_feed' }, { item_id: 303, source: 'rec_cart' }])
  fireEvent.click(screen.getByRole('link', { name: 'Вернуться в меню' }))
  await screen.findByRole('link', { name: new RegExp(dish.name) })
  expect(fetcher.mock.calls.filter(([url]) => url === '/api/v1/menu')).toHaveLength(3)
  vi.unstubAllGlobals()
})
