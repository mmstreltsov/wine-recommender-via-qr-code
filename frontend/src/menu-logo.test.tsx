import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import App from './App'
import { cart, config, dish, json, menu, nextRec, rec } from './test/fixtures'

it('TestFR01_FR05_FR34_AN02_NFR01_brand_link_returns_from_nested_recommendations_and_cart', async () => {
  window.history.replaceState({}, '', '/?table=12')
  const events: { name: string; payload: Record<string, unknown> }[] = []
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (path === '/api/v1/menu?table=12' || path === '/api/v1/menu') return json(menu)
    if (path === '/api/v1/config') return json({ ...config, restaurant: { name: 'АйДаБаран', logo_url: '/static/logo.png' } })
    if (path === '/api/v1/cart') return json(cart)
    if (path === '/api/v1/menu/items/101') return json(dish)
    if (path === '/api/v1/menu/items/202') return json({ ...dish, ...rec.item })
    if (path === '/api/v1/menu/items/303') return json({ ...dish, ...nextRec.item })
    if (path === '/api/v1/menu/items/101/recommendations') return json({ recs: [rec] })
    if (path === '/api/v1/menu/items/202/recommendations') return json({ recs: [nextRec] })
    if (path === '/api/v1/menu/items/303/recommendations') return json({ recs: [] })
    if (path === '/api/v1/events') {
      events.push(JSON.parse(String(init?.body)))
      return new Response(null, { status: 204 })
    }
    throw new Error('Unexpected request')
  })
  vi.stubGlobal('fetch', fetcher)
  render(<App />)

  fireEvent.click(await screen.findByRole('button', { name: 'Основные блюда' }))
  fireEvent.click(screen.getByRole('link', { name: new RegExp(dish.name) }))
  fireEvent.click(await screen.findByRole('link', { name: new RegExp(rec.item.name) }))
  fireEvent.click(await screen.findByRole('link', { name: new RegExp(nextRec.item.name) }))
  await screen.findByRole('heading', { name: nextRec.item.name })
  expect(screen.getByRole('link', { name: '← Назад к блюду' })).toHaveAttribute('href', '/dish/202')
  const brand = screen.getByRole('link', { name: 'АйДаБаран — в меню' })
  expect(brand).toHaveAttribute('href', '/')
  expect(brand.querySelector('img')).toHaveAttribute('alt', '')
  brand.focus()
  expect(brand).toHaveFocus()
  vi.mocked(window.scrollTo).mockClear()
  fireEvent.click(brand)
  await screen.findByRole('link', { name: new RegExp(dish.name) })
  expect(window.location.pathname).toBe('/')
  expect(screen.getByRole('button', { name: 'Все' })).toHaveAttribute('aria-pressed', 'true')
  expect(window.scrollTo).toHaveBeenCalledWith(0, 0)
  expect(screen.getByRole('link', { name: 'Корзина, позиций: 1' })).toBeInTheDocument()
  fireEvent.click(screen.getByRole('link', { name: 'Корзина, позиций: 1' }))
  await screen.findByText('Итого:')
  fireEvent.click(screen.getByRole('link', { name: 'АйДаБаран — в меню' }))
  await screen.findByRole('link', { name: new RegExp(dish.name) })
  expect(screen.getByRole('link', { name: 'Корзина, позиций: 1' })).toBeInTheDocument()
  expect(fetcher.mock.calls.filter(([url]) => url === '/api/v1/menu?table=12')).toHaveLength(1)
  expect(fetcher.mock.calls.some(([url, init]) => String(url).startsWith('/api/v1/cart/items') && init?.method !== undefined && init.method !== 'GET')).toBe(false)
  await waitFor(() => expect(events.filter((event) => event.name === 'dish_card_open').map((event) => event.payload.source)).toEqual(['menu', 'rec_feed', 'rec_feed']))
  vi.unstubAllGlobals()
})
