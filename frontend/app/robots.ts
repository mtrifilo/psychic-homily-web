import { MetadataRoute } from 'next'

export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: '*',
      allow: '/',
      disallow: ['/admin/', '/profile/', '/auth/', '/submissions/', '/library/', '/verify-email/'],
    },
    sitemap: 'https://psychichomily.com/sitemap.xml',
  }
}
