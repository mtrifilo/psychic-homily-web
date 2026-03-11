'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { ProfileSectionResponse } from '@/lib/types/contributor'

interface ProfileSectionsProps {
  sections: ProfileSectionResponse[]
}

export function ProfileSections({ sections }: ProfileSectionsProps) {
  const visibleSections = sections
    .filter(s => s.is_visible)
    .sort((a, b) => a.position - b.position)

  if (visibleSections.length === 0) {
    return null
  }

  return (
    <div className="space-y-4">
      {visibleSections.map(section => (
        <Card key={section.id} className="bg-muted/30 border-border/50">
          <CardHeader className="pb-2">
            <CardTitle className="text-base">{section.title}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="prose prose-sm dark:prose-invert max-w-none">
              <p className="text-sm text-muted-foreground whitespace-pre-wrap">
                {section.content}
              </p>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
