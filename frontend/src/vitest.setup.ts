import '@testing-library/jest-dom/vitest'
import { afterEach, vi } from 'vitest'
import { cleanup } from '@testing-library/react'

export class TestIntersectionObserver implements IntersectionObserver {
  static instances: TestIntersectionObserver[] = []
  readonly root = null
  readonly rootMargin = ''
  readonly thresholds = [0, 0.5, 1]
  readonly targets = new Set<Element>()
  callback: IntersectionObserverCallback

  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback
    TestIntersectionObserver.instances.push(this)
  }
  disconnect() { this.targets.clear() }
  observe(target: Element) { this.targets.add(target) }
  unobserve(target: Element) { this.targets.delete(target) }
  takeRecords(): IntersectionObserverEntry[] { return [] }
  show(target: Element, ratio: number) {
    if (!this.targets.has(target)) return
    this.callback([{ target, intersectionRatio: ratio, isIntersecting: ratio > 0 } as IntersectionObserverEntry], this)
  }
  static show(target: Element, ratio: number) {
    for (const observer of TestIntersectionObserver.instances) observer.show(target, ratio)
  }
}

vi.stubGlobal('IntersectionObserver', TestIntersectionObserver)
if (typeof window !== 'undefined') window.scrollTo = vi.fn()

afterEach(() => {
  cleanup()
  TestIntersectionObserver.instances = []
})
