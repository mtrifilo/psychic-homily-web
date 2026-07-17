import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
  {
    // PSY-868 bundle boundary, enforced mechanically: the homepage sections
    // mount statically and load ForceGraphView in its own dynamic(ssr:false)
    // chunk, so nothing under features/home may VALUE-import the canvas
    // module (type imports are fine and are not flagged by this rule's
    // allowTypeImports behavior below). resolveNodeInVisibleClusters
    // value-imports ForceGraphView, so it is banned transitively too.
    files: ["features/home/**/*.{ts,tsx}"],
    rules: {
      "@typescript-eslint/no-restricted-imports": [
        "error",
        {
          paths: [
            {
              name: "@/components/graph/ForceGraphView",
              message:
                "features/home must not value-import ForceGraphView (PSY-868: it ships in its own dynamic chunk). Use `import type` or createLazyForceGraphView.",
              allowTypeImports: true,
            },
            {
              name: "@/components/graph/resolveNodeInVisibleClusters",
              message:
                "resolveNodeInVisibleClusters value-imports ForceGraphView; importing it from features/home would drag the canvas module into the homepage's initial JS (PSY-868).",
            },
          ],
        },
      ],
    },
  },
]);

export default eslintConfig;
