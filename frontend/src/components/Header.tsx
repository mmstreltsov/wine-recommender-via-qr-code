import { Link } from 'react-router-dom'
import { safeLocalImage } from '../api/validation'

interface Props {
  title: string
  logoUrl: string
  cartCount: number
}

export function Header({ title, logoUrl, cartCount }: Props) {
  return (
    <header className="header">
      <Link to="/" className="header__brand" aria-label={`${title} — в меню`}>
        {safeLocalImage(logoUrl) && <img className="header__logo" src={safeLocalImage(logoUrl)} alt="" onError={(event) => { event.currentTarget.hidden = true }} />}
        <span className="header__title">{title}</span>
      </Link>
      <Link
        to="/cart"
        className="header__cart"
        aria-label={`Корзина${cartCount > 0 ? `, позиций: ${cartCount}` : ''}`}
      >
        <svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true" focusable="false">
          <path
            fill="currentColor"
            d="M7 18a2 2 0 1 0 2 2 2 2 0 0 0-2-2m10 0a2 2 0 1 0 2 2 2 2 0 0 0-2-2M7.2 14.8v.2h11.4l.4-1.9L8.7 12 7.1 5.3A1 1 0 0 0 6.1 4.5H3.9v1.9h1.5L7.5 14l-.3.8m11.9-4.9-12-1.1.4 1.8 10.6 1 .7-1.7z"
          />
        </svg>
        {cartCount > 0 && <span className="header__cart-badge">{cartCount}</span>}
      </Link>
    </header>
  )
}
