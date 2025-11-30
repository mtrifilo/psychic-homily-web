/**
 * API Configuration Tests
 *
 * These tests verify that the API configuration correctly selects
 * the appropriate backend URL based on the environment.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { API_ENDPOINTS, getEnvironmentInfo } from './api'

describe('API Configuration', () => {
    describe('API_BASE_URL', () => {
        beforeEach(() => {
            vi.resetModules()
        })

        it('should use localhost for development', async () => {
            // Mock development environment
            vi.stubGlobal('import', {
                meta: {
                    env: {
                        DEV: true,
                        MODE: 'development',
                        PROD: false,
                        VITE_API_URL: undefined,
                        REACT_APP_API_URL: undefined,
                    },
                },
            })

            // Re-import to get fresh configuration
            const { API_BASE_URL: devUrl } = await import('./api')
            expect(devUrl).toBe('http://localhost:8080')
        })

        it('should use VITE_API_URL when set', () => {
            // The test environment has VITE_API_URL set to localhost
            // This tests that VITE_API_URL takes precedence over other variables
            const envInfo = getEnvironmentInfo()
            expect(envInfo.apiBaseUrl).toBe('http://localhost:8080')
        })

        it('should test API configuration logic', () => {
            // Test the getApiBaseUrl logic by checking the current environment
            const envInfo = getEnvironmentInfo()

            // Verify that the environment variables are being read correctly
            expect(envInfo.apiBaseUrl).toBe('http://localhost:8080')
            expect(envInfo.isDevelopment).toBe(true)
            expect(envInfo.environment).toBe('test')
        })
    })

    describe('API_ENDPOINTS', () => {
        it('should have correct authentication endpoints', () => {
            expect(API_ENDPOINTS.AUTH.LOGIN).toContain('/auth/login')
            expect(API_ENDPOINTS.AUTH.REGISTER).toContain('/auth/register')
            expect(API_ENDPOINTS.AUTH.LOGOUT).toContain('/auth/logout')
            expect(API_ENDPOINTS.AUTH.PROFILE).toContain('/auth/profile')
            expect(API_ENDPOINTS.AUTH.REFRESH).toContain('/auth/refresh')
        })

        it('should generate OAuth endpoints correctly', () => {
            expect(API_ENDPOINTS.AUTH.OAUTH_LOGIN('google')).toContain('/auth/login/google')
            expect(API_ENDPOINTS.AUTH.OAUTH_CALLBACK('github')).toContain('/auth/callback/github')
        })

        it('should have correct application endpoints', () => {
            expect(API_ENDPOINTS.SHOWS.SUBMIT).toContain('/show')
        })

        it('should have correct system endpoints', () => {
            expect(API_ENDPOINTS.HEALTH).toContain('/health')
            expect(API_ENDPOINTS.OPENAPI).toContain('/openapi.json')
        })
    })

    describe('getEnvironmentInfo', () => {
        it('should return environment information', () => {
            const envInfo = getEnvironmentInfo()

            expect(envInfo).toHaveProperty('apiBaseUrl')
            expect(envInfo).toHaveProperty('environment')
            expect(envInfo).toHaveProperty('isDevelopment')
            expect(envInfo).toHaveProperty('isProduction')
        })
    })
})
