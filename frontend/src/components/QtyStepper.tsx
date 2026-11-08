interface Props {
  value: number
  onChange: (qty: number) => void
  min?: number
  max?: number
  disabled?: boolean
}

export function QtyStepper({ value, onChange, min = 1, max = 20, disabled = false }: Props) {
  return (
    <div className="qty-stepper" aria-label="Количество">
      <button
        type="button"
        className="qty-stepper__btn"
        aria-label="Уменьшить количество"
        disabled={disabled || value <= min}
        onClick={() => onChange(value - 1)}
      >
        −
      </button>
      <span className="qty-stepper__value" aria-live="polite">
        {value}
      </span>
      <button
        type="button"
        className="qty-stepper__btn"
        aria-label="Увеличить количество"
        disabled={disabled || value >= max}
        onClick={() => onChange(value + 1)}
      >
        +
      </button>
    </div>
  )
}
