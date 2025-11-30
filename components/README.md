# Psychic Homily Components

## Available Scripts

This project uses `pnpm` as its package manager.

First, install the necessary dependencies:

```bash
pnpm install
```

Once the installation is complete, you can use the following scripts:

### `pnpm run dev`

Starts the development server using Vite. This command is ideal for local development, as it provides features like Hot Module Replacement (HMR) for instant feedback.

### `pnpm run build`

Builds the application for production. This script first runs the TypeScript compiler (`tsc -b`) to ensure there are no type errors, then uses Vite to create an optimized, production-ready build in the `dist` directory.

### `pnpm run lint`

Analyzes the codebase with ESLint to identify and report on patterns, potential errors, and style guide violations.

### `pnpm run preview`

Starts a local server to preview the production build from the `dist` folder. This is a useful way to test the final application before deploying it.
