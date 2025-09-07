/**
 * API Configuration Tests
 *
 * These tests verify that the API configuration correctly selects
 * the appropriate backend URL based on the environment.
 */

import { API_BASE_URL, API_ENDPOINTS, getEnvironmentInfo } from './api'

describe('API Configuration', () => {
    describe('API_BASE_URL', () => {
        it('should use localhost for development', () => {
            // Mock development environment
            const originalEnv = import.meta.env
            Object.defineProperty(import.meta, 'env', {
                value: { ...originalEnv, DEV: true, MODE: 'development' },
                writable: true,
            })

            // Re-import to get fresh configuration
            jest.resetModules()
            const { API_BASE_URL: devUrl } = require('./api')

            expect(devUrl).toBe('http://localhost:8080')
        })

        it('should use production URL when no environment variables are set', () => {
            // Mock production environment
            const originalEnv = import.meta.env
            Object.defineProperty(import.meta, 'env', {
                value: { ...originalEnv, DEV: false, PROD: true, MODE: 'production' },
                writable: true,
            })

            // Re-import to get fresh configuration
            jest.resetModules()
            const { API_BASE_URL: prodUrl } = require('./api')

            expect(prodUrl).toBe('https://api.psychichomily.com')
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
