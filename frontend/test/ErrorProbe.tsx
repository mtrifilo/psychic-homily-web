import { Component, type ReactNode } from 'react'

/**
 * An error boundary that records what reaches it and renders nothing after.
 * Placed where a route's error page would catch, it shows whether a failure
 * escaped the boundaries below it.
 */
export class ErrorProbe extends Component<
  { onCaught: (error: unknown) => void; children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(error: unknown) {
    this.props.onCaught(error)
  }

  render() {
    return this.state.failed ? null : this.props.children
  }
}
