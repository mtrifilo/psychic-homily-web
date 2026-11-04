import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { HOME_SECTIONS } from '../sections'

vi.mock('./HomeLayoutRuntime', () => ({
  HomeLayoutRuntime: ({ sections }: { sections: Record<string, ReactNode> }) => (
    <>
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
vi.mock('./CommunityPulseBand', () => ({ CommunityPulseBand: () => null }))
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

import { SignedInHome } from './SignedInHome'

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
