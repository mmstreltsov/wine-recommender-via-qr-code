import { useEffect, useRef } from 'react'

export function useInView<T extends Element>(onVisible: () => void, scope: number | string): {
  ref: React.RefObject<T>
} {
  const ref = useRef<T>(null)
  const cbRef = useRef(onVisible)
  cbRef.current = onVisible

  useEffect(() => {
    const el = ref.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting && entry.intersectionRatio >= 0.5) {
            observer.disconnect()
            cbRef.current()
          }
        }
      },
      { threshold: [0, 0.5, 1] },
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [scope])

  return { ref }
}
