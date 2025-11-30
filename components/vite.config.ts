/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig(({ mode }) => {
    // Determine environment from mode or fallback to production
    const environment = mode === 'stage' ? 'stage' : 'production'

    return {
        plugins: [react(), tailwindcss()],
        resolve: {
            alias: {
                '@': path.resolve(__dirname, './src'),
            },
        },
        define: {
            'process.env.ENVIRONMENT': `"${environment}"`,
            'process.env.REACT_APP_API_URL':
                environment === 'stage' ? '"https://stage.api.psychichomily.com"' : '"https://api.psychichomily.com"',
        },
        envPrefix: ['VITE_', 'REACT_APP_'],
        build: {
            outDir: '../assets/js',
            sourcemap: environment === 'stage',
            emptyOutDir: true,
            rollupOptions: {
                output: {
                    entryFileNames: 'index.js',
                    chunkFileNames: '[name].js',
                    assetFileNames: '[name].[ext]',
                },
            },
        },
        server: {
            port: environment === 'stage' ? 3001 : 3000,
            host: true,
        },
        test: {
            globals: true,
            environment: 'jsdom',
            setupFiles: ['./src/test/setup.ts'],
        },
    }
})
