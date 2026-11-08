import { Link } from 'react-router-dom'

interface Props {
  onRetry?: () => void
  menuButton?: boolean
}

export function ErrorRetry({ onRetry, menuButton = false }: Props) {
  return (
    <div className="error-retry" role="alert">
      <p className="error-retry__text">Что-то пошло не так, попробуйте ещё раз</p>
      {onRetry && (
        <button type="button" className="btn" onClick={onRetry}>
          Повторить
        </button>
      )}
      {menuButton && (
        <Link className="btn" to="/">
          В меню
        </Link>
      )}
    </div>
  )
}
