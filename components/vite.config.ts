import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
    // Load env file based on `mode` in the current working directory.
    const env = loadEnv(mode, process.cwd(), '')

    // Determine environment from mode or fallback to production
    const environment = mode === 'stage' ? 'stage' : 'production'

    return {
        plugins: [react()],
        define: {
            'process.env.ENVIRONMENT': `"${environment}"`,
            'process.env.NODE_ENV': `"${environment}"`,
            'process.env.REACT_APP_ENV': `"${environment}"`,
            'process.env.REACT_APP_API_URL':
                environment === 'stage' ? '"https://stage.api.psychichomily.com"' : '"https://api.psychichomily.com"',
        },
        build: {
            outDir: environment === 'stage' ? 'dist-stage' : 'dist',
            sourcemap: environment === 'stage',
            rollupOptions: {
                output: {
                    manualChunks: {
                        vendor: ['react', 'react-dom'],
                        form: ['@tanstack/react-form', 'zod'],
                    },
                },
            },
        },
        server: {
            port: environment === 'stage' ? 3001 : 3000,
            host: true,
        },
    }
})
