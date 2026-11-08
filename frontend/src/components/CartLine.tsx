import { formatPrice } from '../app/format'
import type { CartLineData } from '../api/types'
import { PhotoWithFallback } from './PhotoWithFallback'
import { QtyStepper } from './QtyStepper'

interface Props {
  line: CartLineData
  busy?: boolean
  onQtyChange: (qty: number) => void
  onRemove: () => void
}

export function CartLine({ line, busy = false, onQtyChange, onRemove }: Props) {
  return (
    <li
      className={`cart-line${line.available ? '' : ' cart-line--unavailable'}`}
      data-item-id={line.item.id}
    >
      <button
        type="button"
        className="cart-line__remove"
        aria-label={`Удалить «${line.item.name}» из корзины`}
        onClick={onRemove}
        disabled={busy}
      >
        ×
      </button>
      <PhotoWithFallback
        photoUrl={line.item.photo_url}
        photoFallback={line.item.photo_fallback}
        size={200}
        alt={line.item.name}
        className="cart-line__img"
      />
      <div className="cart-line__info">
        <span className="cart-line__name">{line.item.name}</span>
        <span className="cart-line__price">{formatPrice(line.item.price)}{line.qty > 1 ? ` × ${line.qty}` : ''}</span>
        {!line.available && (
          <span className="cart-line__badge" role="status">
            Нет в наличии
          </span>
        )}
      </div>
      <div className="cart-line__controls">
        <QtyStepper value={line.qty} onChange={onQtyChange} disabled={busy} />
      </div>
    </li>
  )
}
