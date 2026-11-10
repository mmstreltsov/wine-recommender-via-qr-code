import { useEffect, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { readLocationPageState, usePageRestore } from '../app/dish-navigation'
import { getCachedConfig, getMenu } from '../api/client'
import type { MenuResponse } from '../api/types'
import { trackCategorySelect, trackMenuViewOnce } from '../api/tracker'
import { useCartCount } from '../app/cart-context'
import { CategoryChips } from '../components/CategoryChips'
import type { ChipId } from '../components/CategoryChips'
import { DishCard } from '../components/DishCard'
import { ErrorRetry } from '../components/ErrorRetry'
import { Header } from '../components/Header'
import type { MenuItemSummary } from '../api/types'

type LoadState = 'loading' | 'error' | 'ready'

export default function MenuPage() {
  const location = useLocation()
  const [state, setState] = useState<LoadState>('loading')
  const [menu, setMenu] = useState<MenuResponse | null>(null)
  const config = getCachedConfig()
  const [attempt, setAttempt] = useState(0)
  const [active, setActive] = useState<ChipId>(() => readLocationPageState(location).view?.category ?? 'all')
  const { count } = useCartCount()
  usePageRestore(state === 'ready' || state === 'error', active)

  useEffect(() => {
    let alive = true
    setState('loading')
    void getMenu().then((menuData) => {
      if (!alive) return
      setMenu(menuData)
      setActive((current) => current === 'all' || menuData.categories.some((category) => category.id === current) ? current : 'all')
      setState('ready')
      trackMenuViewOnce()
    }).catch(() => { if (alive) setState('error') })
    return () => { alive = false }
  }, [attempt])

  const handleSelect = (id: ChipId) => {
    setActive(id)
    trackCategorySelect(id === 'all' ? 0 : id)
  }

  const items: MenuItemSummary[] =
    menu === null
      ? []
      : active === 'all'
        ? menu.categories.flatMap((c) => c.items)
        : (menu.categories.find((c) => c.id === active)?.items ?? [])

  return (
    <main className="page page--menu" aria-busy={state === 'loading'}>
      <Header
        title={config?.restaurant.name ?? 'Онлайн-меню'}
        logoUrl={config?.restaurant.logo_url ?? ''}
        cartCount={count}
      />
      <div className="menu-heading"><p className="eyebrow">Выбирайте, сочетайте, наслаждайтесь</p><h1 id="page-heading" className="page__title" tabIndex={-1}>Онлайн-меню</h1><p>Любимые блюда и немного вдохновения для вашего заказа.</p></div>
      {state === 'loading' && (
        <div className="catalog" aria-hidden="true">
          {Array.from({ length: 6 }, (_, i) => (
            <div key={i} className="skeleton skeleton--card" />
          ))}
        </div>
      )}
      {state === 'error' && <ErrorRetry onRetry={() => setAttempt((value) => value + 1)} />}
      {state === 'ready' && menu && (
        <>
          {menu.categories.length > 0 && (
            <CategoryChips
              categories={menu.categories.map(({ id, name }) => ({ id, name }))}
              activeId={active}
              onSelect={handleSelect}
            />
          )}
          {items.length > 0 ? (
            <div className="catalog">
              {items.map((item) => (
                <DishCard key={item.id} item={item} category={active} />
              ))}
            </div>
          ) : (
            <p className="page__empty">Пока нет доступных позиций</p>
          )}
        </>
      )}
    </main>
  )
}
