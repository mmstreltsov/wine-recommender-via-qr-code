import { createContext, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import { getCart, getLastCart, onCartUpdate } from '../api/client'
import type { CartResponse } from '../api/types'

interface CartCountState {
  count: number
  cart: CartResponse | null
}

const CartCountContext = createContext<CartCountState>({ count: 0, cart: null })

function countItems(lines: { qty: number }[]): number {
  return lines.reduce((sum, line) => sum + line.qty, 0)
}

export function CartCountProvider({ children }: { children: ReactNode }) {
  const [cart, setCart] = useState<CartResponse | null>(getLastCart)
  const { pathname } = useLocation()

  useEffect(() => {
    const unsubscribe = onCartUpdate(setCart)
    return unsubscribe
  }, [])

  useEffect(() => {
    if (pathname !== '/cart' && pathname !== '/pay-stub') void getCart().catch(() => {})
  }, [pathname])

  return <CartCountContext.Provider value={{ count: cart ? countItems(cart.lines) : 0, cart }}>{children}</CartCountContext.Provider>
}

export function useCartCount(): CartCountState {
  return useContext(CartCountContext)
}
