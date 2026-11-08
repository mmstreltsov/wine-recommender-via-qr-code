import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CartResponse } from '../api/types'
import * as api from '../api/client'
import * as tracker from '../api/tracker'
import { CartCountProvider } from '../app/cart-context'
import { cart, config, deferred, dish, emptyCart, menu, nextRec, rec } from '../test/fixtures'
import { TestIntersectionObserver } from '../vitest.setup'
import CartPage from './CartPage'
import DishPage from './DishPage'
import MenuPage from './MenuPage'
import PayStubPage from './PayStubPage'

vi.mock('../api/client', async (original) => ({
  ...await original<typeof api>(),
  getCart: vi.fn(), getMenu: vi.fn(), getConfig: vi.fn(), getItem: vi.fn(), getRecommendations: vi.fn(),
  addItem: vi.fn(), setItemQty: vi.fn(), removeCartItem: vi.fn(), onCartUpdate: vi.fn(), getCachedConfig: vi.fn(), getLastCart: vi.fn(),
}))
vi.mock('../api/tracker', async (original) => ({
  ...await original<typeof tracker>(),
  trackCategorySelect: vi.fn(), trackMenuViewOnce: vi.fn(), trackDishCardOpen: vi.fn(),
  trackCartOpen: vi.fn(), trackPayClick: vi.fn(), trackStubView: vi.fn(), trackRecImpression: vi.fn(),
}))

let listeners: Set<(value: CartResponse) => void>
function publish(value: CartResponse) { for (const listener of listeners) listener(value); return value }
function Location() { return <output>{useLocation().pathname}</output> }
function Jump() {
  const navigate = useNavigate()
  return <button onClick={() => void navigate('/dish/404')}>Другое блюдо</button>
}
function renderPage(path: string, controls = false) {
  return render(<MemoryRouter initialEntries={[path]}><CartCountProvider>
    {controls && <Jump />}
    <Routes><Route path="/" element={<MenuPage />} /><Route path="/dish/:id" element={<DishPage />} /><Route path="/cart" element={<CartPage />} /><Route path="/pay-stub" element={<PayStubPage />} /></Routes>
    <Location />
  </CartCountProvider></MemoryRouter>)
}

beforeEach(() => {
  vi.resetAllMocks()
  listeners = new Set()
  vi.mocked(api.onCartUpdate).mockImplementation((listener) => { listeners.add(listener); return () => { listeners.delete(listener) } })
  vi.mocked(api.getLastCart).mockReturnValue(null)
  vi.mocked(api.getCachedConfig).mockReturnValue(config)
  vi.mocked(api.getConfig).mockResolvedValue(config)
  vi.mocked(api.getMenu).mockResolvedValue(menu)
  vi.mocked(api.getItem).mockResolvedValue(dish)
  vi.mocked(api.getRecommendations).mockResolvedValue({ recs: [rec] })
  vi.mocked(api.getCart).mockImplementation(async () => publish(emptyCart))
  vi.mocked(api.addItem).mockImplementation(async () => publish(cart))
})

describe('Menu', () => {
  it('TestFR25_filter_and_all_chip_event_zero_only_on_user_selection', async () => {
    renderPage('/')
    await screen.findByRole('link', { name: new RegExp(dish.name) })
    expect(tracker.trackCategorySelect).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Хлеб' }))
    expect(screen.queryByRole('link', { name: new RegExp(dish.name) })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: new RegExp(rec.item.name) })).toBeInTheDocument()
    expect(tracker.trackCategorySelect).toHaveBeenLastCalledWith(2)
    fireEvent.click(screen.getByRole('button', { name: 'Все' }))
    expect(screen.getByRole('link', { name: new RegExp(dish.name) })).toBeInTheDocument()
    expect(tracker.trackCategorySelect).toHaveBeenLastCalledWith(0)
  })

  it('TestFR03_FR37_reentry_fetches_menu_again_and_opens_dish', async () => {
    renderPage('/')
    fireEvent.click(await screen.findByRole('link', { name: new RegExp(dish.name) }))
    await screen.findByRole('heading', { name: dish.name })
    fireEvent.click(screen.getByRole('link', { name: '← В меню' }))
    await screen.findByRole('heading', { name: 'Онлайн-меню' })
    await waitFor(() => expect(api.getMenu).toHaveBeenCalledTimes(2))
    expect(api.getItem).toHaveBeenCalledWith(101)
    expect(api.getRecommendations).toHaveBeenCalledWith(101)
  })

  it('TestFR35_menu_error_shows_generic_message_and_retry', async () => {
    vi.mocked(api.getMenu).mockRejectedValueOnce(new Error('SQL secret /private/file'))
    renderPage('/')
    expect(await screen.findByRole('alert')).toHaveTextContent('Что-то пошло не так, попробуйте ещё раз')
    expect(screen.queryByText(/SQL|secret|private/)).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    await screen.findByRole('link', { name: new RegExp(dish.name) })
    expect(api.getMenu).toHaveBeenCalledTimes(2)
  })
})

describe('Dish', () => {
  it.each([
    { kbju: 'Калорийность: 150 ккал', description: '', visible: true },
    { kbju: '', description: 'Овощи и зелень', visible: true },
    { kbju: '', description: '', visible: false },
    { kbju: ' ', description: ' ', visible: false },
  ])('TestFR07_composition_$visible_$description', async ({ visible, ...fields }) => {
    vi.mocked(api.getItem).mockResolvedValue({ ...dish, ...fields })
    renderPage('/dish/101')
    await screen.findByRole('heading', { name: dish.name })
    expect(Boolean(screen.queryByRole('region', { name: 'Состав' }))).toBe(visible)
    const additions = screen.getByRole('region', { name: 'Дополнения' })
    expect(additions).toHaveTextContent('Для информации')
    expect(within(additions).queryByRole('button')).not.toBeInTheDocument()
  })

  it('TestFR29_dish_without_recommendations_has_no_empty_block', async () => {
    vi.mocked(api.getRecommendations).mockResolvedValue({ recs: [] })
    renderPage('/dish/101')
    await screen.findByRole('heading', { name: dish.name })
    expect(screen.queryByText(config.rec_block_title)).not.toBeInTheDocument()
  })

  it('TestFR30_FR28_add_dish_updates_badge_and_refetches_recommendations', async () => {
    renderPage('/dish/101')
    fireEvent.click(await screen.findByRole('button', { name: 'Добавить в корзину' }))
    await screen.findByText('✓ Добавлено')
    expect(api.addItem).toHaveBeenCalledWith(101, 'menu')
    expect(screen.getByRole('link', { name: 'Корзина, позиций: 1' })).toBeInTheDocument()
    await waitFor(() => expect(api.getRecommendations).toHaveBeenCalledTimes(2), { timeout: 3000 })
  })

  it('TestFR35_dish_network_error_retry_and_404_menu_exit', async () => {
    vi.mocked(api.getItem).mockRejectedValueOnce(new Error('Network error'))
    renderPage('/dish/101')
    fireEvent.click(await screen.findByRole('button', { name: 'Повторить' }))
    await screen.findByRole('heading', { name: dish.name })
    expect(tracker.trackDishCardOpen).toHaveBeenCalledTimes(1)
  })

  it('TestFR35_FR37_unavailable_dish_404_has_menu_exit_without_internal_details', async () => {
    vi.mocked(api.getItem).mockRejectedValueOnce(new api.ApiError(404))
    renderPage('/dish/101')
    await screen.findByRole('alert')
    expect(screen.getByRole('link', { name: 'В меню' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Повторить' })).not.toBeInTheDocument()
    expect(screen.queryByText(/404|not_found/)).not.toBeInTheDocument()
  })

  it('TestNFR04_invalid_route_id_is_not_sent_to_API', async () => {
    renderPage('/dish/1e2')
    await screen.findByRole('alert')
    expect(api.getItem).not.toHaveBeenCalled()
    expect(api.getRecommendations).not.toHaveBeenCalled()
  })

  it('TestNFR03_NFR04_composition_is_plain_text_without_forms_or_unsafe_HTML', async () => {
    const content = '<img src=x onerror="alert(1)"><script>alert(1)</script>'
    vi.mocked(api.getItem).mockResolvedValue({ ...dish, description: content })
    const { container } = renderPage('/dish/101')
    expect(await screen.findByText(content)).toBeInTheDocument()
    expect(container.querySelector('script, form, input, textarea')).toBeNull()
    expect(container.querySelector('img[src="x"]')).toBeNull()
  })

  it('TestFR03_AN02_route_id_change_ignores_stale_response_and_opens_new_scope', async () => {
    const first = deferred<typeof dish>()
    vi.mocked(api.getItem).mockImplementation((id) => id === 101 ? first.promise : Promise.resolve({ ...dish, id: 404, name: 'Новое блюдо' }))
    renderPage('/dish/101', true)
    fireEvent.click(screen.getByRole('button', { name: 'Другое блюдо' }))
    await screen.findByRole('heading', { name: 'Новое блюдо' })
    await act(async () => { first.resolve(dish); await first.promise })
    expect(screen.queryByRole('heading', { name: dish.name })).not.toBeInTheDocument()
    expect(tracker.trackDishCardOpen).toHaveBeenCalledWith(404)
    expect(tracker.trackDishCardOpen).not.toHaveBeenCalledWith(101)
  })

  it('TestFR32_dish_add_is_disabled_at_twenty', async () => {
    vi.mocked(api.getCart).mockImplementation(async () => publish({ ...cart, lines: [{ ...cart.lines[0], qty: 20 }], total: 13000 }))
    renderPage('/dish/101')
    expect(await screen.findByRole('button', { name: 'Добавить в корзину' })).toBeDisabled()
  })
})

describe('Cart', () => {
  it('TestFR31_empty_cart_has_neither_payment_nor_recommendations', async () => {
    renderPage('/cart')
    await screen.findByText('Корзина пуста')
    expect(screen.getByRole('link', { name: 'Перейти в меню' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Оплатить' })).not.toBeInTheDocument()
    expect(screen.queryByText(config.rec_block_title)).not.toBeInTheDocument()
  })

  it('TestUI04_FR37_unavailable_line_blocks_payment_without_events_but_qty_and_removal_work', async () => {
    const unavailable = { ...cart, lines: [{ ...cart.lines[0], available: false }] }
    vi.mocked(api.getCart).mockImplementation(async () => publish(unavailable))
    vi.mocked(api.setItemQty).mockImplementation(async () => publish({ ...unavailable, lines: [{ ...unavailable.lines[0], qty: 2 }], total: 1300 }))
    vi.mocked(api.removeCartItem).mockImplementation(async () => publish(emptyCart))
    renderPage('/cart')
    expect(await screen.findByText('Нет в наличии')).toBeInTheDocument()
    expect(screen.getByRole('listitem')).toHaveClass('cart-line--unavailable')
    fireEvent.click(screen.getByRole('button', { name: 'Оплатить' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Часть позиций больше нет в наличии')
    expect(tracker.trackPayClick).not.toHaveBeenCalled()
    expect(tracker.trackStubView).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Увеличить количество' }))
    await waitFor(() => expect(api.setItemQty).toHaveBeenCalledWith(101, 2))
    await screen.findByText('1 300 ₽')
    fireEvent.click(screen.getByRole('button', { name: /Удалить/ }))
    await screen.findByText('Корзина пуста')
    expect(api.removeCartItem).toHaveBeenCalledWith(101)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('TestFR20_rec_cart_add_replaces_lines_total_availability_recommendation_and_badge', async () => {
    vi.mocked(api.getCart).mockImplementation(async () => publish(cart))
    const updated: CartResponse = { lines: [{ ...cart.lines[0], available: false }, { item: rec.item, qty: 1, available: true, added_from: 'rec_cart' }], total: 850, recommendation: nextRec }
    vi.mocked(api.addItem).mockImplementation(async () => publish(updated))
    renderPage('/cart')
    fireEvent.click(await screen.findByRole('button', { name: `Добавить «${rec.item.name}» в корзину` }))
    await screen.findByText(nextRec.item.name)
    expect(api.addItem).toHaveBeenCalledWith(202, 'rec_cart')
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('850 ₽')).toBeInTheDocument()
    expect(screen.getByText('Нет в наличии')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Корзина, позиций: 2' })).toBeInTheDocument()
  })

  it('TestUI04_FR37_qty_change_then_pay_keeps_blocked_feedback_inside_sticky_panel_and_focused', async () => {
    const unavailable = { ...cart, lines: [{ ...cart.lines[0], available: false }] }
    vi.mocked(api.getCart).mockImplementation(async () => publish(unavailable))
    vi.mocked(api.setItemQty).mockImplementation(async () => publish({ ...unavailable, lines: [{ ...unavailable.lines[0], qty: 2 }], total: 1300 }))
    renderPage('/cart')
    fireEvent.click(await screen.findByRole('button', { name: 'Увеличить количество' }))
    await screen.findByText('1 300 ₽')
    const pay = screen.getByRole('button', { name: 'Оплатить' })
    await waitFor(() => expect(pay).toBeEnabled())
    fireEvent.click(pay)
    const feedback = screen.getByRole('alert')
    expect(pay.parentElement).toHaveClass('cart-pay')
    expect(pay.parentElement).toContainElement(feedback)
    expect(feedback).toHaveFocus()
    expect(pay).toHaveAccessibleDescription('Часть позиций больше нет в наличии — уберите их из заказа')
    fireEvent.click(pay)
    expect(feedback).toHaveFocus()
    expect(tracker.trackPayClick).not.toHaveBeenCalled()
    expect(tracker.trackStubView).not.toHaveBeenCalled()
  })

  it('TestUI04_FR20_payment_waits_for_pending_rec_cart_response_before_using_availability', async () => {
    vi.mocked(api.getCart).mockImplementation(async () => publish(cart))
    const response = deferred<CartResponse>()
    vi.mocked(api.addItem).mockImplementation(() => response.promise.then(publish))
    renderPage('/cart')
    fireEvent.click(await screen.findByRole('button', { name: `Добавить «${rec.item.name}» в корзину` }))
    expect(screen.getByRole('button', { name: 'Оплатить' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Оплатить' }))
    expect(tracker.trackPayClick).not.toHaveBeenCalled()
    await act(async () => { response.resolve({ ...cart, lines: [{ ...cart.lines[0], available: false }], recommendation: nextRec }); await response.promise })
    fireEvent.click(screen.getByRole('button', { name: 'Оплатить' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Часть позиций больше нет в наличии')
    expect(tracker.trackPayClick).not.toHaveBeenCalled()
    expect(tracker.trackStubView).not.toHaveBeenCalled()
  })

  it('TestFR35_cart_mutation_error_retry_repeats_last_operation', async () => {
    vi.mocked(api.getCart).mockImplementation(async () => publish(cart))
    vi.mocked(api.removeCartItem).mockRejectedValueOnce(new Error('private database')).mockImplementationOnce(async () => publish(emptyCart))
    renderPage('/cart')
    fireEvent.click(await screen.findByRole('button', { name: /Удалить/ }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Что-то пошло не так')
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    await screen.findByText('Корзина пуста')
    expect(api.removeCartItem).toHaveBeenCalledTimes(2)
  })

  it('TestFR35_cart_load_failure_retries_without_losing_open_scope', async () => {
    vi.mocked(api.getCart).mockRejectedValueOnce(new Error('Offline')).mockImplementationOnce(async () => publish(cart))
    renderPage('/cart')
    fireEvent.click(await screen.findByRole('button', { name: 'Повторить' }))
    await screen.findByRole('button', { name: 'Оплатить' })
    expect(api.getCart).toHaveBeenCalledTimes(2)
    expect(tracker.trackCartOpen).toHaveBeenCalledTimes(1)
  })

  it('TestFR35_cart_qty_failure_keeps_snapshot_and_retries_target_qty', async () => {
    vi.mocked(api.getCart).mockImplementation(async () => publish(cart))
    vi.mocked(api.setItemQty).mockRejectedValueOnce(new Error('Offline')).mockImplementationOnce(async () => publish({ ...cart, lines: [{ ...cart.lines[0], qty: 2 }], total: 1300 }))
    renderPage('/cart')
    fireEvent.click(await screen.findByRole('button', { name: 'Увеличить количество' }))
    await screen.findByRole('alert')
    expect(screen.getByText('650 ₽', { selector: '.cart-total__sum' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    await screen.findByText('1 300 ₽')
    expect(vi.mocked(api.setItemQty).mock.calls).toEqual([[101, 2], [101, 2]])
  })

  it('TestFR14_rec_add_failure_has_retry_and_no_navigation_then_confirmation', async () => {
    vi.mocked(api.getCart).mockImplementation(async () => publish(cart))
    vi.mocked(api.addItem).mockRejectedValueOnce(new Error('Offline')).mockImplementationOnce(async () => publish({ ...cart, recommendation: rec }))
    renderPage('/cart')
    const button = await screen.findByRole('button', { name: `Добавить «${rec.item.name}» в корзину` })
    fireEvent.click(button)
    expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось добавить')
    expect(screen.getByRole('status')).toHaveTextContent('/cart')
    fireEvent.click(button)
    await screen.findByText('✓ Добавлено')
    expect(vi.mocked(api.addItem).mock.calls).toEqual([[202, 'rec_cart'], [202, 'rec_cart']])
  })

  it('TestFR24_payment_tracks_before_navigation_and_shows_exact_stub', async () => {
    vi.mocked(api.getCart).mockImplementation(async () => publish({ ...cart, recommendation: null }))
    renderPage('/cart')
    fireEvent.click(await screen.findByRole('button', { name: 'Оплатить' }))
    await screen.findByText('Ваш выбор сформирован. Позовите официанта.')
    expect(tracker.trackPayClick).toHaveBeenCalledWith(650, 1)
    expect(tracker.trackStubView).toHaveBeenCalledTimes(1)
    expect(vi.mocked(tracker.trackPayClick).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(tracker.trackStubView).mock.invocationCallOrder[0])
  })

  it('TestAN02_cart_impression_uses_same_scope_after_recommendation_change', async () => {
    vi.mocked(api.getCart).mockImplementation(async () => publish(cart))
    renderPage('/cart')
    await screen.findByText(rec.item.name)
    const card = screen.getByText(rec.item.name).closest('.rec-card')!
    act(() => TestIntersectionObserver.show(card, 0.5))
    const scope = vi.mocked(tracker.trackRecImpression).mock.calls[0][0]
    act(() => { publish({ ...cart, recommendation: nextRec }) })
    act(() => TestIntersectionObserver.show(screen.getByText(nextRec.item.name).closest('.rec-card')!, 0.5))
    expect(tracker.trackRecImpression).toHaveBeenLastCalledWith(scope, 'cart', 303, 'heuristic')
    act(() => { publish(cart) })
    act(() => TestIntersectionObserver.show(screen.getByText(rec.item.name).closest('.rec-card')!, 0.5))
    expect(tracker.trackRecImpression).toHaveBeenLastCalledWith(scope, 'cart', 202, 'expert')
    expect(tracker.trackCartOpen).toHaveBeenCalledTimes(1)
  })
})
