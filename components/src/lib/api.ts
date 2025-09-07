/**
 * API Configuration Utility
 *
 * This module provides centralized API configuration that automatically
 * selects the correct backend URL based on the environment.
 */

// Get the API base URL from environment variables
const getApiBaseUrl = (): string => {
    // Check for environment-specific API URL first
    if (import.meta.env.VITE_API_URL) {
        return import.meta.env.VITE_API_URL
    }

    // Fallback to the old React environment variable
    if (import.meta.env.REACT_APP_API_URL) {
        return import.meta.env.REACT_APP_API_URL
    }

    // Development fallback
    if (import.meta.env.DEV) {
        return 'http://localhost:8080'
    }

    // Production fallback
    return 'https://api.psychichomily.com'
}

// Export the configured API base URL
export const API_BASE_URL = getApiBaseUrl()

// API endpoint configuration
export const API_ENDPOINTS = {
    // Authentication endpoints
    AUTH: {
        LOGIN: `${API_BASE_URL}/auth/login`,
        LOGOUT: `${API_BASE_URL}/auth/logout`,
        REGISTER: `${API_BASE_URL}/auth/register`,
        PROFILE: `${API_BASE_URL}/auth/profile`,
        REFRESH: `${API_BASE_URL}/auth/refresh`,
        // OAuth endpoints
        OAUTH_LOGIN: (provider: string) => `${API_BASE_URL}/auth/login/${provider}`,
        OAUTH_CALLBACK: (provider: string) => `${API_BASE_URL}/auth/callback/${provider}`,
    },

    // Application endpoints
    SHOWS: {
        SUBMIT: `${API_BASE_URL}/show`,
        // Add more show-related endpoints as needed
    },

    // System endpoints
    HEALTH: `${API_BASE_URL}/health`,
    OPENAPI: `${API_BASE_URL}/openapi.json`,
} as const

// Utility function to make API requests with proper configuration
export const apiRequest = async <T = any>(endpoint: string, options: RequestInit = {}): Promise<T> => {
    const defaultHeaders: Record<string, string> = {
        'Content-Type': 'application/json',
    }

    const config: RequestInit = {
        credentials: 'include', // Always include cookies for HTTP-only auth
        ...options,
        headers: {
            ...defaultHeaders,
            ...options.headers,
        },
    }

    const response = await fetch(endpoint, config)

    if (!response.ok) {
        console.log('response', response)
        const error = await response.json().catch(() => ({
            message: `HTTP ${response.status}: ${response.statusText}`,
        }))

        // Create a more detailed error object
        const detailedError = {
            message: error.message || `HTTP ${response.status}: ${response.statusText}`,
            status: response.status,
            statusText: response.statusText,
            details: error.details || error.errors || error, // Capture validation errors
        }

        throw new Error(JSON.stringify(detailedError))
    }

    return response.json()
}

// Environment information for debugging
export const getEnvironmentInfo = () => ({
    apiBaseUrl: API_BASE_URL,
    environment: import.meta.env.MODE,
    isDevelopment: import.meta.env.DEV,
    isProduction: import.meta.env.PROD,
})
