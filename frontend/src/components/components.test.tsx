import { act, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { dish, rec } from '../test/fixtures'
import { TestIntersectionObserver } from '../vitest.setup'
import { PhotoWithFallback } from './PhotoWithFallback'
import { RecCard } from './RecCard'
import { RecFeed } from './RecFeed'
import { QtyStepper } from './QtyStepper'
import * as tracker from '../api/tracker'

vi.mock('../api/tracker', async (original) => {
  const actual = await original<typeof tracker>()
  return { ...actual, trackRecImpression: vi.fn(actual.trackRecImpression), trackRecFeedVisible: vi.fn(actual.trackRecFeedVisible) }
})

function Location() { return <output>{useLocation().pathname}</output> }

describe('FR-26 photos', () => {
  it.each([200, 400, 1200] as const)('TestFR26_primary_resize_%s_local_stub_and_prop_reset', (size) => {
    const props = { photoUrl: dish.photo_url, photoFallback: dish.photo_fallback, size, alt: dish.name }
    const { rerender } = render(<PhotoWithFallback {...props} />)
    expect(screen.getByRole('img')).toHaveAttribute('src', `https://static.tildacdn.com/photos/-/resize/${size}x${size}/vegetables.jpg`)
    expect(screen.getByRole('img')).toHaveAttribute('loading', 'lazy')
    fireEvent.error(screen.getByRole('img'))
    expect(screen.getByRole('img')).toHaveAttribute('src', dish.photo_fallback)
    fireEvent.error(screen.getByRole('img'))
    expect(screen.getByRole('img').tagName).toBe('DIV')
    rerender(<PhotoWithFallback {...props} photoUrl="https://static.tildacdn.com/photos/new.jpg" />)
    expect(screen.getByRole('img')).toHaveAttribute('src', `https://static.tildacdn.com/photos/-/resize/${size}x${size}/new.jpg`)
    rerender(<PhotoWithFallback {...props} photoUrl="" photoFallback="" />)
    expect(screen.getByRole('img').tagName).toBe('DIV')
  })

  it('TestNFR04_photo_rejects_unsafe_sources_and_resets_on_size_and_fallback_changes', () => {
    const { rerender } = render(<PhotoWithFallback photoUrl="javascript:alert(1)" photoFallback="//evil.test/image" size={400} alt="Блюдо" />)
    expect(screen.getByRole('img').tagName).toBe('DIV')
    rerender(<PhotoWithFallback photoUrl={dish.photo_url} photoFallback={dish.photo_fallback} size={400} alt="Блюдо" />)
    fireEvent.error(screen.getByRole('img'))
    rerender(<PhotoWithFallback photoUrl={dish.photo_url} photoFallback={dish.photo_fallback} size={1200} alt="Блюдо" />)
    expect(screen.getByRole('img').getAttribute('src')).toContain('/1200x1200/')
    fireEvent.error(screen.getByRole('img'))
    fireEvent.error(screen.getByRole('img'))
    rerender(<PhotoWithFallback photoUrl="" photoFallback="/static/new.jpg" size={1200} alt="Блюдо" />)
    expect(screen.getByRole('img')).toHaveAttribute('src', '/static/new.jpg')
  })

  it.each(['https://tildacdn.com/photos/dish.jpg', 'https://other.tildacdn.net/photos/dish.jpg'])(
    'TestFR26_NFR04_disallowed_CDN_source_uses_local_fallback_%s', (photoUrl) => {
      render(<PhotoWithFallback photoUrl={photoUrl} photoFallback={dish.photo_fallback} size={400} alt={dish.name} />)
      expect(screen.getByRole('img')).toHaveAttribute('src', dish.photo_fallback)
      fireEvent.error(screen.getByRole('img'))
      expect(screen.getByRole('img').tagName).toBe('DIV')
    },
  )
})

describe('Recommendations', () => {
  it.each(['dish', 'cart'] as const)('TestFR15_RI09_recommendation_link_and_sibling_add_%s', (context) => {
    render(<MemoryRouter initialEntries={[context === 'dish' ? '/dish/101' : '/cart']}><RecCard rec={rec} context={context} scope={100} /><Location /></MemoryRouter>)
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '/dish/202')
    expect(link).toContainElement(screen.getByText(rec.item.name))
    expect(link).toContainElement(screen.getByText(rec.reason_text))
    expect(link).toContainElement(screen.getByRole('img'))
    expect(link).not.toContainElement(screen.getByRole('button'))
    expect(link.querySelector('button,a')).toBeNull()
    expect(screen.getByRole('img').getAttribute('src')).toContain('/1200x1200/')
    expect(link.firstElementChild).toHaveClass('rec-card__visual')
    fireEvent.click(link)
    expect(screen.getByRole('status')).toHaveTextContent('/dish/202')
  })

  it('TestFR29_empty_feed_is_hidden', () => {
    render(<RecFeed recs={[]} dishId={101} title="Хорошо сочетается с" scope={101} />)
    expect(screen.queryByRole('region')).not.toBeInTheDocument()
    expect(screen.queryByText('Хорошо сочетается с')).not.toBeInTheDocument()
  })

  it('TestAN02_visibility_observes_compact_heading_at_50_percent_not_entire_feed', () => {
    const { rerender } = render(<RecFeed recs={[rec]} dishId={101} title="Хорошо сочетается с" scope={102} />, { wrapper: MemoryRouter })
    const heading = screen.getByRole('heading')
    const body = screen.getByRole('region')
    expect(TestIntersectionObserver.instances.some((observer) => observer.targets.has(heading))).toBe(true)
    expect(TestIntersectionObserver.instances.some((observer) => observer.targets.has(body))).toBe(false)
    const initial = vi.mocked(tracker.trackRecFeedVisible).mock.calls.length
    act(() => TestIntersectionObserver.show(heading, 0.49))
    expect(tracker.trackRecFeedVisible).toHaveBeenCalledTimes(initial)
    act(() => TestIntersectionObserver.show(heading, 0.5))
    expect(tracker.trackRecFeedVisible).toHaveBeenLastCalledWith(102, 101, 1)
    rerender(<RecFeed recs={[rec, { ...rec, item: { ...rec.item, id: 999 } }]} dishId={101} title="Хорошо сочетается с" scope={102} />)
    act(() => TestIntersectionObserver.show(heading, 1))
    expect(tracker.trackRecFeedVisible).toHaveBeenCalledTimes(initial + 1)
  })

  it('TestFR32_qty_limits_1_and_20', () => {
    const change = vi.fn()
    const { rerender } = render(<QtyStepper value={1} onChange={change} />)
    expect(screen.getByRole('button', { name: 'Уменьшить количество' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Увеличить количество' }))
    expect(change).toHaveBeenCalledWith(2)
    rerender(<QtyStepper value={20} onChange={change} />)
    expect(screen.getByRole('button', { name: 'Увеличить количество' })).toBeDisabled()
  })
})
