import { DishLink } from '../app/dish-navigation'
import { formatPrice } from '../app/format'
import type { MenuItemSummary } from '../api/types'
import { PhotoWithFallback } from './PhotoWithFallback'

export function DishCard({ item, category = 'all' }: { item: MenuItemSummary; category?: number | 'all' }) {
  return (
    <DishLink itemId={item.id} source="menu" category={category} className="dish-card">
      <PhotoWithFallback
        photoUrl={item.photo_url}
        photoFallback={item.photo_fallback}
        size={400}
        alt={item.name}
        className="dish-card__img"
      />
      <div className="dish-card__body">
        <h3 className="dish-card__title">{item.name}</h3>
        <span className="dish-card__price">{formatPrice(item.price)}</span>
      </div>
    </DishLink>
  )
}
