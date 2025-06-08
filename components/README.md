# React + TypeScript + Vite

This template provides a minimal setup to get React working in Vite with HMR and some ESLint rules.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react/README.md) uses [Babel](https://babeljs.io/) for Fast Refresh
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react-swc) uses [SWC](https://swc.rs/) for Fast Refresh

## Expanding the ESLint configuration

If you are developing a production application, we recommend updating the configuration to enable type-aware lint rules:

```js
export default tseslint.config({
    extends: [
        // Remove ...tseslint.configs.recommended and replace with this
        ...tseslint.configs.recommendedTypeChecked,
        // Alternatively, use this for stricter rules
        ...tseslint.configs.strictTypeChecked,
        // Optionally, add this for stylistic rules
        ...tseslint.configs.stylisticTypeChecked,
    ],
    languageOptions: {
        // other options...
        parserOptions: {
            project: ['./tsconfig.node.json', './tsconfig.app.json'],
            tsconfigRootDir: import.meta.dirname,
        },
    },
})
```

You can also install [eslint-plugin-react-x](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-x) and [eslint-plugin-react-dom](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-dom) for React-specific lint rules:

```js
// eslint.config.js
import reactX from 'eslint-plugin-react-x'
import reactDom from 'eslint-plugin-react-dom'

export default tseslint.config({
    plugins: {
        // Add the react-x and react-dom plugins
        'react-x': reactX,
        'react-dom': reactDom,
    },
    rules: {
        // other rules...
        // Enable its recommended typescript rules
        ...reactX.configs['recommended-typescript'].rules,
        ...reactDom.configs.recommended.rules,
    },
})
```

Proposed file structure:

src/
├── components/
│ ├── show-form/
│ │ ├── ShowForm.tsx
│ │ ├── ShowFormSchema.ts
│ │ └── types.ts
│ └── ui/
│ ├── Button.tsx
│ └── Input.tsx
├── lib/
│ ├── api.ts
│ └── utils.ts
├── routes/
│ ├── submit-show.tsx
│ └── index.tsx
├── hooks/
│ └── useShowSubmission.ts
└── types/
└── show.ts

## Integrating components with Hugo

Add these to your Hugo templates where you want the React components to appear:

`single.html`

```html
{{ define "main" }}
<div class="container mx-auto px-4 py-8">
    <h1>Submit a Show</h1>
    <!-- React mount point -->
    <div id="show-submission"></div>
</div>

<!-- Include the React bundle -->
<script type="module" src="/js/components.js"></script>
{{ end }}
```

When adding new components, be sure to update index.html to include the mount points for the new components, like:

```html
<div id="show-submission"></div>
<div id="new-component"></div>
```
