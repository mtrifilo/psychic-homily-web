import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  experimental: {
    // Optimize barrel imports for common libraries
    // Impact: 15-70% faster dev boot, 28% faster builds, 40% faster cold starts
    optimizePackageImports: [
      'lucide-react',
      '@radix-ui/react-icons',
      '@radix-ui/react-dialog',
      '@radix-ui/react-dropdown-menu',
      '@radix-ui/react-tabs',
      '@radix-ui/react-tooltip',
      '@radix-ui/react-popover',
      '@radix-ui/react-select',
      '@radix-ui/react-slot',
      'date-fns',
      '@tanstack/react-query',
    ],
  },
};

export default nextConfig;
