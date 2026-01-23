import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  experimental: {
    // Optimize barrel imports for common libraries
    // Only list packages that are actually installed
    optimizePackageImports: [
      'lucide-react',
      '@radix-ui/react-dialog',
      '@radix-ui/react-dropdown-menu',
      '@radix-ui/react-tabs',
      '@radix-ui/react-slot',
      '@radix-ui/react-label',
      '@tanstack/react-query',
    ],
  },
};

export default nextConfig;
