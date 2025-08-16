import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
    plugins: [react()],
    mode: 'staging',
    define: {
        'process.env.NODE_ENV': '"staging"',
        'process.env.REACT_APP_ENV': '"staging"',
        'process.env.REACT_APP_API_URL': '"https://stage.api.psychichomily.com"',
    },
    build: {
        outDir: 'dist-staging',
        sourcemap: true,
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
        port: 3001,
        host: true,
    },
})
