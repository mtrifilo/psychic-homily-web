'use client'

import { useState, useEffect } from 'react'
import { Loader2, Users, Inbox, Search } from 'lucide-react'
import { useAdminUsers } from '@/lib/hooks/admin/useAdminUsers'
import { AdminUserCard } from '@/app/admin/users/_components/AdminUserCard'
import { AdminEmptyState } from '@/components/admin'
import { Input } from '@/components/ui/input'

export default function AdminUsersPage() {
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')

  // Debounce search input
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchInput])

  const { data, isLoading, error } = useAdminUsers({
    search: debouncedSearch,
  })

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search by email or username..."
            value={searchInput}
            onChange={e => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center">
        <p className="text-destructive">
          {error instanceof Error
            ? error.message
            : 'Failed to load users. Please try again.'}
        </p>
      </div>
    )
  }

  const users = data?.users || []

  return (
    <div className="space-y-4">
      {/* Search */}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search by email or username..."
          value={searchInput}
          onChange={e => setSearchInput(e.target.value)}
          className="pl-9"
        />
      </div>

      {users.length === 0 ? (
        <AdminEmptyState
          icon={Inbox}
          title="No Users Found"
          message={
            debouncedSearch
              ? `No users match "${debouncedSearch}". Try a different search.`
              : 'No users registered yet.'
          }
        />
      ) : (
        <>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Users className="h-4 w-4" />
            <span>
              {data?.total} user{data?.total !== 1 ? 's' : ''}
              {debouncedSearch && ` matching "${debouncedSearch}"`}
            </span>
          </div>

          <div className="space-y-2">
            {users.map(user => (
              <AdminUserCard key={user.id} user={user} />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
