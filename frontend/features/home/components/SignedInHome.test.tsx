import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { HOME_SECTIONS } from '../sections'

vi.mock('./HomeLayoutRuntime', () => ({
  HomeLayoutRuntime: ({
    sections,
    masthead,
  }: {
    sections: Record<string, ReactNode>
    masthead?: ReactNode
  }) => (
    <>
      <div data-testid="masthead-slot">{masthead}</div>
      {Object.entries(sections).map(([id, node]) => (
        <div key={id} data-testid={`slot-${id}`}>
          {node}
        </div>
      ))}
    </>
  ),
}))
vi.mock('./SavedShowsModule', () => ({ SavedShowsModule: () => null }))
vi.mock('./LibraryUpdates', () => ({ LibraryUpdates: () => null }))
vi.mock('./NearbyShowsSection', () => ({ NearbyShowsSection: () => null }))
vi.mock('./HomeSceneGraph', () => ({
  HomeSceneGraph: ({ narrowForm }: { narrowForm: string }) => (
    <div data-testid="scene-graph" data-narrow-form={narrowForm} />
  ),
}))
vi.mock('./LatestRadioShows', () => ({
  LatestRadioShows: ({ variant }: { variant: string }) => (
    <div data-testid="radio" data-variant={variant} />
  ),
}))

vi.mock('./HomeNameplate', () => ({
  HomeNameplate: () => <div data-testid="nameplate" />,
}))

import { SignedInHome } from './SignedInHome'

describe('SignedInHome masthead and sections', () => {
  it('mounts the nameplate as the masthead, outside the sections', () => {
    render(<SignedInHome />)

    expect(screen.getByTestId('masthead-slot')).toContainElement(
      screen.getByTestId('nameplate')
    )
  })

  // The stats band's counts moved into the dateline; no slot renders it.
  it('hands over exactly the registry sections, without the stats band', () => {
    render(<SignedInHome />)

    const slots = screen
      .getAllByTestId(/^slot-/)
      .map(slot => slot.dataset.testid?.replace('slot-', ''))
    expect(slots.sort()).toEqual(HOME_SECTIONS.map(section => section.id).sort())
  })
})

describe('SignedInHome section forms', () => {
  it('mounts the radio rows and the phone link row for the graph', () => {
    render(<SignedInHome />)

    expect(screen.getByTestId('radio')).toHaveAttribute('data-variant', 'rows')
    expect(screen.getByTestId('scene-graph')).toHaveAttribute(
      'data-narrow-form',
      'linkRow'
    )
  })

  // The Customize row promises the phone link row; the copy and the mounted
  // form must change together.
  it('describes the graph section as the form it mounts', () => {
    render(<SignedInHome />)

    const graph = HOME_SECTIONS.find(section => section.id === 'city_graph')
    expect(graph?.description).toMatch(/a link on phones/i)
    expect(screen.getByTestId('scene-graph')).toHaveAttribute(
      'data-narrow-form',
      'linkRow'
    )
  })
})
