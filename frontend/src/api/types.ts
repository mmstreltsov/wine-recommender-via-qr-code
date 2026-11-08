export interface MenuItemSummary {
  id: number
  category_id: number
  name: string
  price: number
  photo_url: string
  photo_fallback: string
}

export interface UpsellOption {
  name: string
  add_price: number
}

export interface MenuItemFull extends MenuItemSummary {
  description: string
  kbju: string
  upsell_options: UpsellOption[]
}

export interface CategoryWithItems {
  id: number
  name: string
  items: MenuItemSummary[]
}

export interface MenuResponse {
  categories: CategoryWithItems[]
}

export type RecSource = 'expert' | 'upsell' | 'heuristic'

export interface Recommendation {
  item: MenuItemSummary
  reason_text: string
  source: RecSource
}

export interface RecsResponse {
  recs: Recommendation[]
}

export type AddSource = 'menu' | 'rec_feed' | 'rec_cart'

export interface CartLineData {
  item: MenuItemSummary
  qty: number
  added_from: AddSource
  available: boolean
}

export interface CartResponse {
  lines: CartLineData[]
  total: number
  recommendation: Recommendation | null
}

export interface ConfigResponse {
  restaurant: {
    name: string
    logo_url: string
  }
  rec_block_title: string
}
