import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ChartModule, ChartRow } from './ChartModule'

describe('ChartModule', () => {
  it('keeps cached rows visible when a background refetch fails', () => {
    render(
      <ChartModule
        title="Cached chart"
        context="quarter"
        rowCount={1}
        isLoading={false}
        isError
        hasData
        testId="cached-chart"
      >
        <ChartRow
          rank={1}
          primary="Still here"
          meta="Cached metadata"
          action={<button>Follow</button>}
        />
      </ChartModule>
    )

    expect(screen.getByText('Still here')).toBeInTheDocument()
    expect(
      screen.queryByText('Unable to load this chart.')
    ).not.toBeInTheDocument()
  })

  it('shows a blocking error when no data has loaded', () => {
    render(
      <ChartModule
        title="Broken chart"
        context="quarter"
        rowCount={0}
        isLoading={false}
        isError
        hasData={false}
        testId="broken-chart"
      >
        {null}
      </ChartModule>
    )
    expect(screen.getByText('Unable to load this chart.')).toBeInTheDocument()
  })
})
