import { useEffect, useRef, useState } from 'react'
import { addItem } from '../api/client'
import { trackRecImpression } from '../api/tracker'
import type { AddSource, Recommendation } from '../api/types'
import { useInView } from '../app/useInView'
import { formatPrice } from '../app/format'
import { useCartCount } from '../app/cart-context'
import { PhotoWithFallback } from './PhotoWithFallback'

interface Props {
  rec: Recommendation
  context: 'dish' | 'cart'
  scope: number
  disabled?: boolean
  onPendingChange?: (pending: boolean) => void
}

const ADDED_RESET_MS = 1500

export function RecCard({ rec, context, scope, disabled = false, onPendingChange }: Props) {
  const { cart } = useCartCount()
  const [added, setAdded] = useState(false)
  const [pending, setPending] = useState(false)
  const [failed, setFailed] = useState(false)
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const alive = useRef(true)

  const { ref } = useInView<HTMLDivElement>(
    () => trackRecImpression(scope, context, rec.item.id, rec.source),
    `${scope}:${context}:${rec.item.id}`,
  )

  useEffect(
    () => {
      alive.current = true
      return () => {
        alive.current = false
        if (resetTimer.current) clearTimeout(resetTimer.current)
      }
    },
    [],
  )

  async function handleAdd() {
    if (pending || added || disabled) return
    setPending(true)
    onPendingChange?.(true)
    setFailed(false)
    const source: AddSource = context === 'dish' ? 'rec_feed' : 'rec_cart'
    try {
      await addItem(rec.item.id, source)
      if (!alive.current) return
      setAdded(true)
      resetTimer.current = setTimeout(() => setAdded(false), ADDED_RESET_MS)
    } catch {
      if (alive.current) setFailed(true)
    } finally {
      onPendingChange?.(false)
      if (alive.current) setPending(false)
    }
  }

  return (
    <div className="rec-card" ref={ref} data-item-id={rec.item.id}>
      <PhotoWithFallback
        photoUrl={rec.item.photo_url}
        photoFallback={rec.item.photo_fallback}
        size={400}
        alt={rec.item.name}
        className="rec-card__img"
      />
      <div className="rec-card__body">
        <span className="rec-card__title">{rec.item.name}</span>
        <span className="rec-card__price">{formatPrice(rec.item.price)}</span>
        <p className="rec-card__reason">{rec.reason_text}</p>
      </div>
      <div className="rec-card__action">
        <button
          type="button"
          className="btn btn--sm"
          onClick={handleAdd}
          disabled={disabled || pending || added || (cart?.lines.find((line) => line.item.id === rec.item.id)?.qty ?? 0) >= 20}
          aria-label={`Добавить «${rec.item.name}» в корзину`}
        >
          {added ? '✓ Добавлено' : 'Добавить'}
        </button>
        {failed && <span className="rec-card__error" role="alert">Не удалось добавить. Попробуйте ещё раз.</span>}
      </div>
    </div>
  )
}
