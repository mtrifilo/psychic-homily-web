import { afterEach, describe, expect, it, vi } from 'vitest'
import { isDevApiProxyEnabled } from './dev-api-proxy'

afterEach(() => {
  vi.unstubAllEnvs()
})

describe('isDevApiProxyEnabled', () => {
  it.each([
    { nodeEnv: 'development', override: undefined, expected: true },
    { nodeEnv: 'production', override: undefined, expected: false },
    { nodeEnv: 'test', override: undefined, expected: false },
    { nodeEnv: 'production', override: '1', expected: true },
    { nodeEnv: 'production', override: 'true', expected: false },
    { nodeEnv: 'production', override: '0', expected: false },
    { nodeEnv: 'production', override: '', expected: false },
  ])(
    'NODE_ENV=$nodeEnv ENABLE_DEV_API_PROXY=$override is $expected',
    ({ nodeEnv, override, expected }) => {
      vi.stubEnv('NODE_ENV', nodeEnv)
      vi.stubEnv('ENABLE_DEV_API_PROXY', override)

      expect(isDevApiProxyEnabled()).toBe(expected)
    }
  )
})
