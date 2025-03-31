import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'

// https://vite.dev/config/
export default defineConfig({
    plugins: [react()],
    build: {
        // Only use lib configuration for production builds
        ...(process.env.NODE_ENV === 'production'
            ? {
                  lib: {
                      entry: 'src/main.tsx',
                      name: 'PsychicHomilyComponents',
                      fileName: 'components',
                      formats: ['es'],
                  },
                  rollupOptions: {
                      external: ['react', 'react-dom'],
                      output: {
                          globals: {
                              react: 'React',
                              'react-dom': 'ReactDOM',
                          },
                      },
                  },
              }
            : {}),
    },
})
