import path from 'path'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
    plugins: [react(), tailwindcss()],
    resolve: {
        alias: {
            '@': path.resolve(__dirname, './src'),
        },
    },
    build: {
        outDir: '../static/js',
        rollupOptions: {
            output: {
                entryFileNames: 'submit-form.js',
                chunkFileNames: 'submit-form-[hash].js',
                assetFileNames: 'submit-form-[hash].[ext]',
            },
        },
    },
})
