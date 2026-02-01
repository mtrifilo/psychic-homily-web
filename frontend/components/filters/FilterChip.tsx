interface FilterChipProps {
  label: string
  isActive: boolean
  onClick: () => void
  count?: number
}

export function FilterChip({ label, isActive, onClick, count }: FilterChipProps) {
  return (
    <button
      onClick={onClick}
      className={`px-3 py-1.5 rounded-full text-sm font-medium transition-colors duration-[50ms] ${
        isActive
          ? 'bg-primary text-primary-foreground'
          : 'bg-muted hover:bg-muted/80 text-muted-foreground hover:text-foreground'
      }`}
    >
      {label}
      {count !== undefined && (
        <span className={`ml-1.5 ${isActive ? 'opacity-80' : 'opacity-60'}`}>
          ({count})
        </span>
      )}
    </button>
  )
}
