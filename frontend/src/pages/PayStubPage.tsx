import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { trackStubView } from '../api/tracker'

export default function PayStubPage() {
  useEffect(() => {
    trackStubView()
  }, [])

  return (
    <main className="page page--stub">
      <div className="stub">
        <p className="stub__text">Ваш выбор сформирован. Позовите официанта.</p>
        <Link to="/" className="btn">
          Вернуться в меню
        </Link>
      </div>
    </main>
  )
}
