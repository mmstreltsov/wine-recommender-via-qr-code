import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useParams } from 'react-router-dom'
import { DishBackLink, readPageState, usePageRestore } from '../app/dish-navigation'
import { ApiError, addItem, getCachedConfig, getItem, getRecommendations, onCartUpdate } from '../api/client'
import type { MenuItemFull, Recommendation } from '../api/types'
import { nextImpressionScope, trackDishCardOpen } from '../api/tracker'
import { useCartCount } from '../app/cart-context'
import { formatPrice } from '../app/format'
import { ErrorRetry } from '../components/ErrorRetry'
import { Header } from '../components/Header'
import { PhotoWithFallback } from '../components/PhotoWithFallback'
import { RecFeed } from '../components/RecFeed'

type LoadState = 'loading' | 'error' | 'notfound' | 'ready'
type AddState = 'idle' | 'busy' | 'added'

const ADDED_RESET_MS = 1500

export default function DishPage() {
  const { id } = useParams()
  const location = useLocation()
  return <DishDetails key={`${location.key}:${id}`} itemId={id && /^[1-9]\d*$/.test(id) ? Number(id) : NaN} />
}

function DishDetails({ itemId }: { itemId: number }) {
  const location = useLocation()
  const source = readPageState(location.state).entry?.source ?? 'menu'
  const { count, cart } = useCartCount()
  const config = getCachedConfig()

  const [state, setState] = useState<LoadState>('loading')
  const [item, setItem] = useState<MenuItemFull | null>(null)
  const [recs, setRecs] = useState<Recommendation[]>([])
  const [addState, setAddState] = useState<AddState>('idle')
  const [addFailed, setAddFailed] = useState(false)
  const trackedRef = useRef(false)
  const [scope] = useState(nextImpressionScope)
  const requestRef = useRef(0)
  const alive = useRef(true)
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  usePageRestore(state !== 'loading')

  const load = useCallback(async () => {
    if (!Number.isSafeInteger(itemId) || itemId < 1) {
      setState('notfound')
      return
    }
    setState('loading')
    const request = ++requestRef.current
    try {
      const [itemData, recsData] = await Promise.all([getItem(itemId), getRecommendations(itemId)])
      if (!alive.current || request !== requestRef.current) return
      setItem(itemData)
      setRecs(recsData.recs)
      setState('ready')
      if (!trackedRef.current) {
        trackedRef.current = true
        trackDishCardOpen(itemId, source)
      }
    } catch (err) {
      if (!alive.current || request !== requestRef.current) return
      setState(err instanceof ApiError && err.status === 404 ? 'notfound' : 'error')
    }
  }, [itemId, source])

  useEffect(() => {
    alive.current = true
    void load()
    return () => { alive.current = false; requestRef.current += 1 }
  }, [load])

  useEffect(() => {
    if (state !== 'ready') return
    let active = true
    let revision = 0
    let refreshTimer: ReturnType<typeof setTimeout> | undefined
    const unsubscribe = onCartUpdate((updated) => {
      const current = ++revision
      clearTimeout(refreshTimer)
      refreshTimer = setTimeout(() => {
        setRecs((previous) => previous.filter((rec) => !updated.lines.some((line) => line.item.id === rec.item.id)))
        void getRecommendations(itemId).then((data) => {
          if (active && current === revision) setRecs(data.recs)
        }).catch(() => {})
      }, ADDED_RESET_MS)
    })
    return () => { active = false; clearTimeout(refreshTimer); unsubscribe() }
  }, [itemId, state])

  useEffect(
    () => () => {
      if (resetTimer.current) clearTimeout(resetTimer.current)
    },
    [],
  )

  async function handleAddDish() {
    if (!item || addState !== 'idle') return
    setAddState('busy')
    setAddFailed(false)
    try {
      await addItem(item.id, source)
      if (!alive.current) return
      setAddState('added')
      resetTimer.current = setTimeout(() => setAddState('idle'), ADDED_RESET_MS)
    } catch {
      if (!alive.current) return
      setAddState('idle')
      setAddFailed(true)
    }
  }

  return (
    <main className="page page--dish" aria-busy={state === 'loading'}>
      <Header title={config.restaurant.name} logoUrl={config.restaurant.logo_url} cartCount={count} />
      <DishBackLink />
      {state === 'loading' && (
        <div aria-hidden="true">
          <div className="skeleton skeleton--hero" />
          <div className="skeleton skeleton--line" />
          <div className="skeleton skeleton--line" />
        </div>
      )}
      {(state === 'error' || state === 'notfound') && (
        <ErrorRetry menuButton {...(state === 'error' ? { onRetry: load } : {})} />
      )}
      {state === 'ready' && item && (
        <article className="dish">
          <PhotoWithFallback
            photoUrl={item.photo_url}
            photoFallback={item.photo_fallback}
            size={1200}
            alt={item.name}
            className="dish__img"
          />
          <div className="dish__head">
            <h1 id="page-heading" className="dish__title" tabIndex={-1}>{item.name}</h1>
            <div className="dish__buy">
              <span className="dish__price">{formatPrice(item.price)}</span>
              <button
                type="button"
                className="btn"
                onClick={handleAddDish}
                disabled={addState !== 'idle' || (cart?.lines.find((line) => line.item.id === item.id)?.qty ?? 0) >= 20}
              >
                {addState === 'added' ? '✓ Добавлено' : 'Добавить в корзину'}
              </button>
            </div>
            {addFailed && (
              <p className="dish__add-error" role="alert">
                Не удалось добавить позицию
              </p>
            )}
          </div>
            {(item.kbju.trim() || item.description.trim()) && (
            <section className="dish__composition" aria-label="Состав">
              <h2 className="dish__section-title">Состав</h2>
              {item.kbju && <p className="dish__kbju">{item.kbju}</p>}
              {item.description && <p className="dish__descr">{item.description}</p>}
            </section>
          )}
          {item.upsell_options.length > 0 && (
            <section className="dish__upsell" aria-label="Дополнения">
              <h2 className="dish__section-title">Дополнения</h2>
              <p className="muted">Для информации. Дополнения можно обсудить с официантом.</p>
              <ul className="dish__upsell-list">
                {item.upsell_options.map((opt) => (
                  <li key={opt.name} className="dish__upsell-item">
                    <span>{opt.name}</span>
                    <span>+{formatPrice(opt.add_price)}</span>
                  </li>
                ))}
              </ul>
            </section>
          )}
          <RecFeed
            dishId={item.id}
            title={config.rec_block_title}
            recs={recs}
            scope={scope}
          />
        </article>
      )}
    </main>
  )
}
