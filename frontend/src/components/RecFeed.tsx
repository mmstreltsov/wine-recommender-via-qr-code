import type { Recommendation } from '../api/types'
import { trackRecFeedVisible } from '../api/tracker'
import { useInView } from '../app/useInView'
import { RecCard } from './RecCard'

interface Props {
  dishId: number
  title: string
  recs: Recommendation[]
  scope: number
}

export function RecFeed({ dishId, title, recs, scope }: Props) {
  const { ref } = useInView<HTMLHeadingElement>(
    () => trackRecFeedVisible(scope, dishId, recs.length),
    `${scope}:${recs.length > 0}`,
  )

  if (recs.length === 0) return null

  return (
    <section className="rec-feed" aria-label={title}>
      <h2 className="rec-feed__title" ref={ref} tabIndex={-1}>{title}</h2>
      <div className="rec-feed__list">
        {recs.map((rec) => (
          <RecCard key={rec.item.id} rec={rec} context="dish" scope={scope} />
        ))}
      </div>
    </section>
  )
}
