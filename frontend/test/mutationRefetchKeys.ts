import { act, renderHook } from '@testing-library/react'
import type { QueryClient, QueryKey } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'

/** Lets refetchAfterMutation's cancel step settle so its invalidation lands. */
export function settleRefetch(): Promise<void> {
  return act(() => new Promise<void>(resolve => setTimeout(resolve, 0)))
}

/**
 * Runs one mutation against a client whose queries are only seeded (no
 * observers, so nothing refetches over the network), then lets the refetch
 * helper's cancel step settle so its invalidation has landed.
 */
export async function runMutation<TVariables>(
  client: QueryClient,
  useMutationHook: () => {
    mutateAsync: (variables: TVariables) => Promise<unknown>
  },
  variables: TVariables
): Promise<void> {
  const { result, unmount } = renderHook(useMutationHook, {
    wrapper: createWrapperWithClient(client),
  })
  await act(() => result.current.mutateAsync(variables))
  await settleRefetch()
  unmount()
}

/** One mutation with its variables, ready to run against a client. */
export function mutationCase<TVariables>(
  useMutationHook: () => {
    mutateAsync: (variables: TVariables) => Promise<unknown>
  },
  variables: TVariables
) {
  return (client: QueryClient) => runMutation(client, useMutationHook, variables)
}

/** The seeded keys a mutation marked invalidated, in seeding order. */
export function invalidatedKeys(client: QueryClient, seeded: QueryKey[]) {
  return seeded.filter(key => client.getQueryState(key)?.isInvalidated)
}
