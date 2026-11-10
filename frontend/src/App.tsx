import { useCallback, useEffect, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { CartCountProvider } from './app/cart-context'
import { ensureStarted, getConfig } from './api/client'
import { ErrorRetry } from './components/ErrorRetry'
import CartPage from './pages/CartPage'
import DishPage from './pages/DishPage'
import MenuPage from './pages/MenuPage'
import PayStubPage from './pages/PayStubPage'

function ManualScrollRestoration() {
  useEffect(() => {
    const previous = window.history.scrollRestoration
    window.history.scrollRestoration = 'manual'
    return () => { window.history.scrollRestoration = previous }
  }, [])
  return null
}

export default function App() {
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [attempt, setAttempt] = useState(0)
  const retry = useCallback(() => setAttempt((value) => value + 1), [])
  useEffect(() => {
    let alive = true
    setState('loading')
    void ensureStarted().then(getConfig).then((config) => {
      if (!alive) return
      document.title = `${config.restaurant.name} — онлайн-меню`
      setState('ready')
    }).catch(() => { if (alive) setState('error') })
    return () => { alive = false }
  }, [attempt])

  return (
    <BrowserRouter>
      {state === 'loading' && <main className="page" role="status">Загружаем меню…<div className="skeleton skeleton--hero" aria-hidden="true" /></main>}
      {state === 'error' && <main className="page"><ErrorRetry onRetry={retry} /></main>}
      {state === 'ready' && (
        <CartCountProvider>
          <ManualScrollRestoration />
          <Routes>
            <Route path="/" element={<MenuPage />} />
            <Route path="/dish/:id" element={<DishPage />} />
            <Route path="/cart" element={<CartPage />} />
            <Route path="/pay-stub" element={<PayStubPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </CartCountProvider>
      )}
    </BrowserRouter>
  )
}
