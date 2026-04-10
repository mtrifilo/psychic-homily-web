'use client'

import { Library } from 'lucide-react'
import { useUserPublicCollections } from '../hooks'
import { CollectionCard } from './CollectionCard'
import { Card, CardContent } from '@/components/ui/card'

interface UserCollectionsProps {
  username: string
}

export function UserCollections({ username }: UserCollectionsProps) {
  const { data, isLoading } = useUserPublicCollections(username)

  const collections = data?.collections ?? []

  if (isLoading) return null

  if (collections.length === 0) {
    return (
      <Card className="bg-muted/30 border-border/50">
        <CardContent className="p-6 text-center">
          <Library className="h-8 w-8 text-muted-foreground/40 mx-auto mb-2" />
          <p className="text-sm text-muted-foreground">
            No collections yet
          </p>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-3">
      {collections.map(collection => (
        <CollectionCard key={collection.id} collection={collection} />
      ))}
    </div>
  )
}
