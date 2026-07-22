'use client'

import { useState, useEffect, useCallback } from 'react'
import * as Sentry from '@sentry/nextjs'
import { browserSupportsWebAuthn } from '@simplewebauthn/browser'
import { Trash2, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { PasskeyRegisterButton } from '@/features/auth'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

interface PasskeyCredential {
  id: number
  display_name: string
  created_at: string
  last_used_at: string | null
  backup_eligible: boolean
  backup_state: boolean
}

export function PasskeyManagement() {
  const [credentials, setCredentials] = useState<PasskeyCredential[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<number | null>(null)

  const supportsWebAuthn = browserSupportsWebAuthn()

  const fetchCredentials = useCallback(async () => {
    try {
      const response = await fetch(`${API_BASE_URL}/auth/passkey/credentials`, {
        credentials: 'include',
      })
      const data = await response.json()

      if (data.success) {
        setCredentials(data.credentials || [])
      } else {
        setError(data.message || 'Failed to load passkeys')
      }
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'passkey-management', error_type: 'fetch_failed' },
      })
      setError('Failed to load passkeys')
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    if (supportsWebAuthn) {
      fetchCredentials()
    } else {
      setIsLoading(false)
    }
  }, [supportsWebAuthn, fetchCredentials])

  const handleDelete = async (credentialId: number) => {
    if (!confirm('Are you sure you want to remove this passkey?')) {
      return
    }

    setDeletingId(credentialId)
    setError(null)

    try {
      const response = await fetch(`${API_BASE_URL}/auth/passkey/credentials/${credentialId}`, {
        method: 'DELETE',
        credentials: 'include',
      })
      const data = await response.json()

      if (data.success) {
        setCredentials(prev => prev.filter(c => c.id !== credentialId))
      } else {
        setError(data.message || 'Failed to delete passkey')
      }
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'passkey-management', error_type: 'delete_failed' },
        extra: { credentialId },
      })
      setError('Failed to delete passkey')
    } finally {
      setDeletingId(null)
    }
  }

  const formatDate = (dateString: string | null) => {
    if (!dateString) return null
    return new Date(dateString).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    })
  }

  if (!supportsWebAuthn) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Passkeys</CardTitle>
          <CardDescription>Your browser does not support passkeys.</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Passkeys</CardTitle>
        <CardDescription>
          Sign in with Touch ID, Face ID, or a security key.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {error && (
          <Alert variant="destructive" className="mb-4">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {isLoading ? (
          <div className="flex items-center justify-center py-4">
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0 flex-1 space-y-3">
              {credentials.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No passkeys registered yet. Add a passkey for faster, more secure sign-in.
                </p>
              ) : (
                credentials.map(credential => {
                  const lastUsed = formatDate(credential.last_used_at)
                  return (
                    <div
                      key={credential.id}
                      className="flex items-center justify-between gap-3"
                    >
                      <div className="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
                        <p className="text-sm font-medium">
                          {credential.display_name || 'Unnamed passkey'}
                        </p>
                        {lastUsed && (
                          <p className="font-mono text-xs text-muted-foreground">
                            Last used {lastUsed}
                          </p>
                        )}
                        {credential.backup_state && (
                          <span className="text-xs text-success-foreground">Synced</span>
                        )}
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleDelete(credential.id)}
                        disabled={deletingId === credential.id}
                        className="shrink-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        aria-label={`Remove ${credential.display_name || 'passkey'}`}
                      >
                        {deletingId === credential.id ? (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        ) : (
                          <Trash2 className="h-4 w-4" />
                        )}
                      </Button>
                    </div>
                  )
                })
              )}
            </div>
            <PasskeyRegisterButton
              onSuccess={() => fetchCredentials()}
              onError={err => setError(err)}
            />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
