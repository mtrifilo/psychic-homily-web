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
    // Vendored maplibre-gl worker modules (PSY-1538): byte-identical copies
    // of upstream dist files (pinned by maplibreVendored.test.ts) — not our
    // code to lint.
    "public/maplibre/**",
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
  {
    // On /atlas, chrome links that mount before the visitor's first pointerdown,
    // keydown or wheel must hold their prefetches until the map is up, which
    // ChromeLink does (components/layout/nav/ChromeLink.tsx). That includes the
    // links of hover-opened menus (BrowseMenu, ContributeMenu). This rule covers
    // the chrome under components/layout. Links inside a surface that only
    // opens on click or key (the notification bell's list) mount after one of
    // those inputs, which has already released the hold. A later config block
    // that sets no-restricted-imports for these files replaces this one's
    // options rather than merging them, so extend this block instead.
    files: ["components/layout/**/*.tsx"],
    ignores: ["components/layout/**/*.test.tsx"],
    rules: {
      "@typescript-eslint/no-restricted-imports": [
        "error",
        {
          paths: [
            {
              name: "next/link",
              // Link only: useLinkStatus reads the status of the ChromeLink
              // it renders inside, so the chrome may import it.
              importNames: ["default"],
              message:
                "Use ChromeLink from @/components/layout/nav/ChromeLink: it sets the chrome's prefetch policy (the /atlas hold, intent prefetch on touch).",
              allowTypeImports: true,
            },
          ],
        },
      ],
    },
  },
  {
    // The components the Atlas draws in its own pane link through
    // AtlasPaneLink, which holds their prefetches on /atlas until the map is
    // up. The pane's files sit beside the scene page's in this folder, so they
    // are named one by one; atlasPaneLinks.test.ts fails when a component
    // AtlasGlobe reaches in this folder is missing from the list. A component
    // from outside this folder rendered inside the pane is outside this rule.
    files: [
      "features/scenes/components/{ArtistPanel,AtlasGlobe,AtlasSceneList,AtlasSearch,GenreLegend,GlobeCanvas,MyScenesStrip,SceneNotifyModeToggle,ScenePreviewContent,ScenePreviewPanel,VenueListSheet,VenuePanel,VenueRail}.tsx",
    ],
    rules: {
      "@typescript-eslint/no-restricted-imports": [
        "error",
        {
          paths: [
            {
              name: "next/link",
              message:
                "Use AtlasPaneLink from ./AtlasPaneLink: it holds the prefetch on /atlas until the map is up.",
              allowTypeImports: true,
            },
          ],
        },
      ],
    },
  },
  {
    // Mutation hooks refetch through refetchAfterMutation
    // (lib/refetchAfterMutation.ts), which cancels before it invalidates. A
    // bare invalidateQueries or refetchQueries hands back a query's in-flight
    // first fetch, whose response can predate the write, and its success
    // clears the invalidation. The createInvalidateQueries entries go through
    // the helper too, apart from `auth`, which carries its own annotation.
    // The selectors see member calls, optional and computed; a method pulled
    // off the client by destructuring is not caught. The rule covers the hooks
    // trees plus the files named here that hold a mutation's refetch outside
    // them (refetchAfterMutation.lint.test.ts checks every file that imports
    // the helper is covered). A site that is safe without the cancel carries
    // an eslint-disable-next-line comment giving the reason. A later block
    // that sets no-restricted-syntax for these files replaces these selectors
    // rather than merging with them, so extend this block instead.
    files: [
      "features/**/hooks/**",
      "lib/hooks/**",
      "lib/queryClient.ts",
      "app/admin/radio/_components/RadioManagement.tsx",
      "components/shared/AddToCollectionButton.tsx",
      "features/admin/components/DiscoveryTriage/useDiscoveryTriage.ts",
      "features/admin/components/StreamingWorklist/useStreamingWorklist.ts",
      "features/artists/components/ArtistDetail.tsx",
      "features/festivals/components/FestivalDetail.tsx",
      "features/labels/components/LabelDetail.tsx",
      "features/releases/components/ReleaseDetail.tsx",
      "features/scenes/components/SceneNotifyModeToggle.tsx",
      "features/shows/components/ShowDetail.tsx",
      "features/shows/components/ShowSubmissionsConsole.tsx",
      "features/tags/admin/useAdminTags.ts",
      "features/venues/components/VenueCard.tsx",
      "features/venues/components/VenueDetail.tsx",
    ],
    ignores: [
      // Tests drive the cache directly to set up and assert refetch order.
      "**/*.test.ts",
      "**/*.test.tsx",
    ],
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector:
            "CallExpression[callee.property.name=/^(invalidate|refetch)Queries$/]",
          message:
            "Refetch through refetchAfterMutation from @/lib/refetchAfterMutation: a bare invalidateQueries or refetchQueries reuses an in-flight first fetch whose response can predate the write. If this site is safe without the cancel, disable this line and say why.",
        },
        {
          selector:
            "CallExpression[callee.computed=true][callee.property.value=/^(invalidate|refetch)Queries$/]",
          message:
            "Refetch through refetchAfterMutation from @/lib/refetchAfterMutation: a bare invalidateQueries or refetchQueries reuses an in-flight first fetch whose response can predate the write. If this site is safe without the cancel, disable this line and say why.",
        },
      ],
    },
  },
]);

export default eslintConfig;
