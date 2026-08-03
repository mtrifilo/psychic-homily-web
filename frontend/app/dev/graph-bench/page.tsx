'use client'

/** PSY-1724 spike route. THROWAWAY bench, never merged. */

import dynamic from 'next/dynamic'

const Bench = dynamic(() => import('./Bench'), { ssr: false })

export default function GraphBenchPage() {
  return <Bench />
}
