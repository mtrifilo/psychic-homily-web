import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import Loading from './loading'

describe('Admin route loading boundary (app/admin/loading.tsx)', () => {
  it('renders a spinner', () => {
    const { container } = render(<Loading />)

    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })
})
