import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { buildResizedUrl } from '../components/PhotoWithFallback'
import { safeLocalImage, safeRemoteImage } from './validation'

describe('NFR-04 FR-26 CDN allowlist', () => {
  it.each(['static.tildacdn.com', 'optim.tildacdn.com', 'static.tildacdn.net', 'optim.tildacdn.net'])(
    'TestNFR04_accepts_exact_CSP_host_%s', (host) => {
      const url = `https://${host}/photos/dish.jpg`
      expect(safeRemoteImage(url)).toBe(url)
      expect(buildResizedUrl(url, 400)).toBe(`https://${host}/photos/-/resize/400x400/dish.jpg`)
    },
  )

  it.each([
    'https://tildacdn.com/dish.jpg',
    'https://tildacdn.net/dish.jpg',
    'https://other.tildacdn.com/dish.jpg',
    'https://other.tildacdn.net/dish.jpg',
    'https://nested.static.tildacdn.com/dish.jpg',
    'https://nested.optim.tildacdn.net/dish.jpg',
    'https://static.tildacdn.com.evil.test/dish.jpg',
    'https://static.tildacdn.com./dish.jpg',
    'https://static-tildacdn.com/dish.jpg',
    'https://static.tildacdn.com@evil.test/dish.jpg',
    'http://static.tildacdn.com/dish.jpg',
    'https://user:pass@static.tildacdn.com/dish.jpg',
    'https://optim.tildacdn.net:8443/dish.jpg',
    'https://static.tildacdn.com/dish.jpg?tracking=1',
    'https://static.tildacdn.com/dish.jpg#fragment',
    '//static.tildacdn.com/dish.jpg',
    'javascript:alert(1)',
    'data:image/svg+xml,evil',
  ])('TestNFR04_rejects_source_outside_CSP_%s', (url) => {
    expect(safeRemoteImage(url)).toBe('')
    expect(buildResizedUrl(url, 400)).toBe('')
  })

  it('TestFR26_NFR04_all_125_concrete_menu_photos_remain_allowed_at_every_size', () => {
    const menuSource = readFileSync('../data/aidabaran/menu.json', 'utf8')
    const menu: { items: { photos: string[]; editions: { img: string }[]; local_photo: string }[] } = JSON.parse(menuSource)
    expect(menu.items).toHaveLength(125)
    const photos = menu.items.flatMap((item) => [...item.photos, ...item.editions.map((edition) => edition.img)]).filter(Boolean)
    expect(photos.length).toBeGreaterThanOrEqual(125)
    expect([...new Set(photos.map((url) => new URL(url).hostname))]).toEqual(['static.tildacdn.com'])
    for (const photo of photos) {
      expect(safeRemoteImage(photo)).toBe(photo)
      for (const size of [200, 400, 1200] as const) {
        const resized = buildResizedUrl(photo, size)
        expect(resized).toContain(`/-/resize/${size}x${size}/`)
        expect(safeRemoteImage(resized)).toBe(resized)
      }
    }
    for (const item of menu.items) {
      expect(item.photos[0] || item.editions[0]?.img).toBeTruthy()
      const fallback = item.local_photo.replace(/^photos\//, '/static/aidabaran/photos/')
      expect(safeLocalImage(fallback)).toBe(fallback)
    }
  })
})
