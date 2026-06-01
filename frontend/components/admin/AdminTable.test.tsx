import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { AdminTable, type AdminTableColumn } from './AdminTable'

interface Row {
  id: number
  name: string
  count: number
}

const rows: Row[] = [
  { id: 1, name: 'Alpha', count: 3 },
  { id: 2, name: 'Beta', count: 0 },
]

const columns: AdminTableColumn<Row>[] = [
  { key: 'name', header: 'Name', render: r => r.name },
  { key: 'count', header: 'Count', align: 'center', render: r => r.count },
]

describe('AdminTable', () => {
  it('renders column headers and a cell per row via render()', () => {
    render(<AdminTable columns={columns} rows={rows} rowKey={r => r.id} />)
    expect(screen.getByRole('columnheader', { name: 'Name' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Count' })).toBeInTheDocument()
    expect(screen.getByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('Beta')).toBeInTheDocument()
    // 1 header row + 2 data rows
    expect(screen.getAllByRole('row')).toHaveLength(3)
  })

  it('applies the editorial mono-uppercase header treatment', () => {
    render(<AdminTable columns={columns} rows={rows} rowKey={r => r.id} />)
    const header = screen.getByRole('columnheader', { name: 'Name' })
    expect(header.className).toContain('font-mono')
    expect(header.className).toContain('uppercase')
  })

  it('aligns columns per the align prop', () => {
    render(<AdminTable columns={columns} rows={rows} rowKey={r => r.id} />)
    expect(screen.getByRole('columnheader', { name: 'Count' }).className).toContain('text-center')
  })

  it('calls onRowClick on click; clickable rows are focusable + cursor-affordanced', () => {
    const onRowClick = vi.fn()
    render(<AdminTable columns={columns} rows={rows} rowKey={r => r.id} onRowClick={onRowClick} />)
    const row = screen.getByText('Alpha').closest('tr')!
    expect(row.className).toContain('cursor-pointer')
    expect(row).toHaveAttribute('tabindex', '0')
    fireEvent.click(row)
    expect(onRowClick).toHaveBeenCalledWith(rows[0])
  })

  it('activates onRowClick via keyboard (Enter / Space) when the row is focused', () => {
    const onRowClick = vi.fn()
    render(<AdminTable columns={columns} rows={rows} rowKey={r => r.id} onRowClick={onRowClick} />)
    const row = screen.getByText('Alpha').closest('tr')!
    fireEvent.keyDown(row, { key: 'Enter' })
    fireEvent.keyDown(row, { key: ' ' })
    expect(onRowClick).toHaveBeenCalledTimes(2)
    expect(onRowClick).toHaveBeenCalledWith(rows[0])
  })

  it('does NOT activate the row when a focused child control receives the key', () => {
    const onRowClick = vi.fn()
    const cols: AdminTableColumn<Row>[] = [
      { key: 'name', header: 'Name', render: r => r.name },
      { key: 'toggle', header: 'Toggle', stopRowClick: true, render: () => <button>toggle</button> },
    ]
    render(<AdminTable columns={cols} rows={rows} rowKey={r => r.id} onRowClick={onRowClick} />)
    // Key event originates on the child button (target !== the row), so the
    // row's Enter/Space handler must not fire onRowClick.
    fireEvent.keyDown(screen.getAllByText('toggle')[0], { key: 'Enter' })
    expect(onRowClick).not.toHaveBeenCalled()
  })

  it('does not add clickable affordance (cursor/tabindex) when onRowClick is absent', () => {
    render(<AdminTable columns={columns} rows={rows} rowKey={r => r.id} />)
    const row = screen.getByText('Alpha').closest('tr')!
    expect(row.className).not.toContain('cursor-pointer')
    expect(row).not.toHaveAttribute('tabindex')
  })

  it('passes align=right and cellClassName through to cells', () => {
    const cols: AdminTableColumn<Row>[] = [
      { key: 'count', header: 'Count', align: 'right', cellClassName: 'text-muted-foreground', render: r => r.count },
    ]
    render(<AdminTable columns={cols} rows={[rows[0]]} rowKey={r => r.id} />)
    const cell = screen.getByText('3').closest('td')!
    expect(cell.className).toContain('text-right')
    expect(cell.className).toContain('text-muted-foreground')
  })

  it('stopRowClick cells do not trigger the row click', () => {
    const onRowClick = vi.fn()
    const cols: AdminTableColumn<Row>[] = [
      { key: 'name', header: 'Name', render: r => r.name },
      {
        key: 'toggle',
        header: 'Toggle',
        stopRowClick: true,
        render: () => <button>toggle</button>,
      },
    ]
    render(<AdminTable columns={cols} rows={rows} rowKey={r => r.id} onRowClick={onRowClick} />)
    fireEvent.click(screen.getAllByText('toggle')[0])
    expect(onRowClick).not.toHaveBeenCalled()
  })

  it('applies rowClassName (e.g. a selected highlight)', () => {
    render(
      <AdminTable
        columns={columns}
        rows={rows}
        rowKey={r => r.id}
        rowClassName={r => (r.id === 1 ? 'bg-muted/50' : undefined)}
      />
    )
    expect(screen.getByText('Alpha').closest('tr')!.className).toContain('bg-muted/50')
    expect(screen.getByText('Beta').closest('tr')!.className).not.toContain('bg-muted/50')
  })

  it('renders the empty slot (spanning all columns) when there are no rows', () => {
    render(
      <AdminTable
        columns={columns}
        rows={[]}
        rowKey={(r: Row) => r.id}
        empty={<span>Nothing here</span>}
      />
    )
    const cell = screen.getByText('Nothing here').closest('td')!
    expect(cell).toHaveAttribute('colspan', String(columns.length))
  })

  it('supports rich cell content (badges, multi-line) via render()', () => {
    const cols: AdminTableColumn<Row>[] = [
      {
        key: 'name',
        header: 'Name',
        render: r => (
          <div>
            <div data-testid="title">{r.name}</div>
            <div data-testid="sub">id-{r.id}</div>
          </div>
        ),
      },
    ]
    render(<AdminTable columns={cols} rows={[rows[0]]} rowKey={r => r.id} />)
    const row = screen.getByText('Alpha').closest('tr')!
    expect(within(row).getByTestId('sub')).toHaveTextContent('id-1')
  })
})
