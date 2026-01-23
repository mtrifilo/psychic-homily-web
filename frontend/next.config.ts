import type { NextConfig } from "next";
import path from "path";

const nextConfig: NextConfig = {
  webpack: (config) => {
    config.resolve.alias = {
      ...config.resolve.alias,
      '@': path.resolve(process.cwd()),
    };
    return config;
  },
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
