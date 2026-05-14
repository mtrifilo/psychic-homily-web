import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { DenseTable, DenseTableGroupHeader } from './DenseTable'

describe('DenseTable', () => {
  it('renders a table with the given rows', () => {
    render(
      <DenseTable>
        <thead>
          <tr>
            <th>Title</th>
            <th>Year</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>Heart Under</td>
            <td>2022</td>
          </tr>
        </tbody>
      </DenseTable>
    )
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByText('Heart Under')).toBeInTheDocument()
    expect(screen.getByText('2022')).toBeInTheDocument()
  })

  it('renders thead column headers as <th> cells', () => {
    render(
      <DenseTable>
        <thead>
          <tr>
            <th>Title</th>
            <th>Year</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>x</td>
            <td>x</td>
          </tr>
        </tbody>
      </DenseTable>
    )
    expect(screen.getByRole('columnheader', { name: 'Title' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Year' })).toBeInTheDocument()
  })

  it('forwards custom className', () => {
    const { container } = render(
      <DenseTable className="mt-4">
        <tbody>
          <tr>
            <td>x</td>
          </tr>
        </tbody>
      </DenseTable>
    )
    const table = container.querySelector('table')
    expect(table?.className).toContain('mt-4')
  })

  it('forwards arbitrary table HTML attributes (e.g. aria-label)', () => {
    render(
      <DenseTable aria-label="Discography">
        <tbody>
          <tr>
            <td>x</td>
          </tr>
        </tbody>
      </DenseTable>
    )
    expect(screen.getByRole('table', { name: 'Discography' })).toBeInTheDocument()
  })
})

describe('DenseTableGroupHeader', () => {
  it('renders the title inside the table body', () => {
    render(
      <DenseTable>
        <tbody>
          <DenseTableGroupHeader title="Albums & EPs" colSpan={3} />
          <tr>
            <td colSpan={3}>row</td>
          </tr>
        </tbody>
      </DenseTable>
    )
    expect(screen.getByText('Albums & EPs')).toBeInTheDocument()
  })

  it('renders as a <th scope="rowgroup"> for accessibility', () => {
    render(
      <DenseTable>
        <tbody>
          <DenseTableGroupHeader title="Singles" colSpan={3} />
          <tr>
            <td colSpan={3}>row</td>
          </tr>
        </tbody>
      </DenseTable>
    )
    const th = screen.getByText('Singles').closest('th')
    expect(th).toHaveAttribute('scope', 'rowgroup')
  })

  it('spans the given number of columns', () => {
    render(
      <DenseTable>
        <tbody>
          <DenseTableGroupHeader title="Singles" colSpan={4} />
          <tr>
            <td colSpan={4}>row</td>
          </tr>
        </tbody>
      </DenseTable>
    )
    const th = screen.getByText('Singles').closest('th')
    expect(th).toHaveAttribute('colspan', '4')
  })

  it('forwards custom className onto the wrapping tr', () => {
    render(
      <DenseTable>
        <tbody>
          <DenseTableGroupHeader
            title="Production"
            colSpan={2}
            className="custom-row-cls"
          />
          <tr>
            <td colSpan={2}>row</td>
          </tr>
        </tbody>
      </DenseTable>
    )
    const tr = screen.getByText('Production').closest('tr')
    expect(tr?.className).toContain('custom-row-cls')
  })
})
