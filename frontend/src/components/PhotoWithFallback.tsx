import { useState } from 'react'
import { safeLocalImage, safeRemoteImage } from '../api/validation'

type PhotoSize = 200 | 400 | 1200

interface Props {
  photoUrl: string
  photoFallback: string
  size: PhotoSize
  alt: string
  className?: string
}

export function buildResizedUrl(url: string, size: PhotoSize): string {
  const safe = safeRemoteImage(url)
  if (!safe) return ''
  const parsed = new URL(safe)
  const segments = parsed.pathname.replace(/\/-\/resize\/\d+x\d+\//g, '/').split('/')
  const file = segments.pop()
  if (!file) return ''
  parsed.pathname = [...segments, '-', 'resize', `${size}x${size}`, file].join('/')
  return parsed.href
}

export function PhotoWithFallback(props: Props) {
  return <Photo key={`${props.photoUrl}:${props.photoFallback}:${props.size}`} {...props} />
}

function Photo({ photoUrl, photoFallback, size, alt, className }: Props) {
  const primary = buildResizedUrl(photoUrl, size)
  const local = safeLocalImage(photoFallback)
  const sources = [primary, local].filter(Boolean)
  const [index, setIndex] = useState(0)
  const src = sources[index]

  if (!src) {
    return (
      <div className={`photo-stub${className ? ` ${className}` : ''}`} role="img" aria-label={alt}>
        <svg viewBox="0 0 24 24" width="40" height="40" aria-hidden="true" focusable="false">
          <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" />
          <circle cx="12" cy="12" r="5" fill="none" stroke="currentColor" />
        </svg>
      </div>
    )
  }
  return <img key={src} className={className} src={src} alt={alt} loading="lazy" referrerPolicy="no-referrer" onError={() => setIndex((value) => value + 1)} />
}
