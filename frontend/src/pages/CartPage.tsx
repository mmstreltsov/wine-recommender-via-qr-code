import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { getCart, getCachedConfig, onCartUpdate, removeCartItem, setItemQty } from '../api/client'
import type { CartResponse } from '../api/types'
import { nextImpressionScope, trackCartOpen, trackPayClick } from '../api/tracker'
import { useCartCount } from '../app/cart-context'
import { usePageRestore } from '../app/dish-navigation'
import { formatPrice } from '../app/format'
import { CartLine } from '../components/CartLine'
import { ErrorRetry } from '../components/ErrorRetry'
import { Header } from '../components/Header'
import { RecCard } from '../components/RecCard'

type LoadState = 'loading' | 'error' | 'ready'

export default function CartPage() {
  const { count } = useCartCount()
  const config = getCachedConfig()
  const navigate = useNavigate()

  const [state, setState] = useState<LoadState>('loading')
  const [cart, setCart] = useState<CartResponse | null>(null)
  const [blockedMsg, setBlockedMsg] = useState(false)
  const [busyId, setBusyId] = useState<number | null>(null)
  const [recPending, setRecPending] = useState(false)
  const [scope] = useState(nextImpressionScope)
  const [mutationError, setMutationError] = useState(false)
  const retryMutation = useRef<(() => Promise<void>) | null>(null)
  const alive = useRef(true)
  const revision = useRef(0)
  const opened = useRef(false)
  const blockedFeedback = useRef<HTMLParagraphElement>(null)
  usePageRestore(state !== 'loading')

  const load = useCallback(async () => {
    setState('loading')
    const current = ++revision.current
    try {
      const cartData = await getCart()
      if (!alive.current || current !== revision.current) return
      setCart(cartData)
      setState('ready')
      if (!opened.current) {
        opened.current = true
        trackCartOpen(cartData.lines.length, cartData.total)
      }
    } catch {
      if (!alive.current || current !== revision.current) return
      setState('error')
    }
  }, [])

  useEffect(() => {
    alive.current = true
    const unsubscribe = onCartUpdate(setCart)
    void load()
    return () => { alive.current = false; revision.current += 1; unsubscribe() }
  }, [load])

  const hasUnavailable = cart?.lines.some((line) => !line.available) ?? false

  useEffect(() => {
    if (!hasUnavailable) setBlockedMsg(false)
    else if (blockedMsg) blockedFeedback.current?.focus({ preventScroll: true })
  }, [hasUnavailable, blockedMsg])

  async function handleQty(item_id: number, qty: number) {
    if (busyId !== null) return
    setBusyId(item_id)
    setMutationError(false)
    try {
      await setItemQty(item_id, qty)
    } catch {
      if (alive.current) {
        retryMutation.current = () => handleQty(item_id, qty)
        setMutationError(true)
      }
    } finally {
      if (alive.current) setBusyId(null)
    }
  }

  async function handleRemove(item_id: number) {
    if (busyId !== null) return
    setBusyId(item_id)
    setMutationError(false)
    try {
      await removeCartItem(item_id)
    } catch {
      if (alive.current) {
        retryMutation.current = () => handleRemove(item_id)
        setMutationError(true)
      }
    } finally {
      if (alive.current) setBusyId(null)
    }
  }

  function handlePay() {
    if (!cart || cart.lines.length === 0 || busyId !== null || recPending) return
    if (cart.lines.some((line) => !line.available)) {
      setBlockedMsg(true)
      blockedFeedback.current?.focus({ preventScroll: true })
      return
    }
    trackPayClick(cart.total, cart.lines.length)
    navigate('/pay-stub')
  }

  const isEmpty = cart !== null && cart.lines.length === 0

  return (
    <main className="page page--cart" aria-busy={state === 'loading'}>
      <Header title={config.restaurant.name} logoUrl={config.restaurant.logo_url} cartCount={count} />
      <Link to="/" className="back-link">
        ← В меню
      </Link>
      <h1 id="page-heading" className="page__title" tabIndex={-1}>Корзина</h1>
      {state === 'loading' && (
        <div aria-hidden="true">
          {Array.from({ length: 2 }, (_, i) => (
            <div key={i} className="skeleton skeleton--line" />
          ))}
        </div>
      )}
      {state === 'error' && <ErrorRetry onRetry={load} />}
      {mutationError && <ErrorRetry onRetry={() => void retryMutation.current?.()} />}
      {state === 'ready' && cart && isEmpty && (
        <div className="cart-empty">
          <div className="cart-empty__plate" aria-hidden="true">
            <svg viewBox="0 0 24 24" width="48" height="48" focusable="false">
              <path
                fill="currentColor"
                d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2m0 18a8 8 0 1 1 8-8 8 8 0 0 1-8 8m-1-13h2v5h-2zm0 7h2v2h-2z"
              />
            </svg>
          </div>
          <p className="cart-empty__text">Корзина пуста</p>
          <Link to="/" className="btn">
            Перейти в меню
          </Link>
        </div>
      )}
      {state === 'ready' && cart && !isEmpty && (
        <>
          <ul className="cart-list">
            {cart.lines.map((line) => (
              <CartLine
                key={line.item.id}
                line={line}
                busy={busyId !== null || recPending}
                onQtyChange={(qty) => void handleQty(line.item.id, qty)}
                onRemove={() => void handleRemove(line.item.id)}
              />
            ))}
          </ul>
          <div className="cart-total">
            <span>Итого:</span>
            <span className="cart-total__sum">{formatPrice(cart.total)}</span>
          </div>
          {cart.recommendation && (
            <section className="rec-feed rec-feed--cart" aria-label={config.rec_block_title}>
              <h2 className="rec-feed__title" tabIndex={-1}>{config.rec_block_title}</h2>
              <div className="rec-feed__list">
                <RecCard key={cart.recommendation.item.id} rec={cart.recommendation} context="cart" scope={scope} disabled={busyId !== null} onPendingChange={setRecPending} />
              </div>
            </section>
          )}
          <div className="cart-pay">
            {blockedMsg && hasUnavailable && (
              <p id="cart-payment-feedback" className="cart-blocked-msg" role="alert" tabIndex={-1} ref={blockedFeedback}>
                Часть позиций больше нет в наличии — уберите их из заказа
              </p>
            )}
            <button type="button" className="btn btn--pay" onClick={handlePay} disabled={busyId !== null || recPending} aria-describedby={blockedMsg && hasUnavailable ? 'cart-payment-feedback' : undefined}>
              Оплатить
            </button>
          </div>
        </>
      )}
    </main>
  )
}
