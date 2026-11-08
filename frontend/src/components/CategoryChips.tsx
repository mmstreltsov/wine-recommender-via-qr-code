export type ChipId = 'all' | number

export interface ChipCategory {
  id: number
  name: string
}

interface Props {
  categories: ChipCategory[]
  activeId: ChipId
  onSelect: (id: ChipId) => void
}

export function CategoryChips({ categories, activeId, onSelect }: Props) {
  return (
    <nav className="chip-row" aria-label="Категории меню">
      <button
        type="button"
        className={`chip${activeId === 'all' ? ' is-active' : ''}`}
        aria-pressed={activeId === 'all'}
        onClick={() => onSelect('all')}
      >
        Все
      </button>
      {categories.map((c) => (
        <button
          key={c.id}
          type="button"
          className={`chip${activeId === c.id ? ' is-active' : ''}`}
          aria-pressed={activeId === c.id}
          onClick={() => onSelect(c.id)}
        >
          {c.name}
        </button>
      ))}
    </nav>
  )
}
