'use client'

import { useState, useEffect, useCallback } from 'react'
import { browserSupportsWebAuthn } from '@simplewebauthn/browser'
import { Fingerprint, Trash2, Loader2, Shield, Clock } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { PasskeyRegisterButton } from '@/components/auth/passkey-register'

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
    } catch {
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
    } catch {
      setError('Failed to delete passkey')
    } finally {
      setDeletingId(null)
    }
  }

  const formatDate = (dateString: string | null) => {
    if (!dateString) return 'Never'
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
          <CardTitle className="flex items-center gap-2">
            <Fingerprint className="h-5 w-5" />
            Passkeys
          </CardTitle>
          <CardDescription>Your browser does not support passkeys.</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Fingerprint className="h-5 w-5" />
              Passkeys
            </CardTitle>
            <CardDescription className="mt-1">
              Passkeys let you sign in securely without a password using biometrics or a security
              key.
            </CardDescription>
          </div>
          <PasskeyRegisterButton
            onSuccess={() => fetchCredentials()}
            onError={err => setError(err)}
          />
        </div>
      </CardHeader>
      <CardContent>
        {error && (
          <Alert variant="destructive" className="mb-4">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : credentials.length === 0 ? (
          <div className="text-center py-8">
            <Shield className="h-12 w-12 mx-auto text-muted-foreground mb-3" />
            <p className="text-sm text-muted-foreground">
              No passkeys registered yet. Add a passkey for faster, more secure sign-in.
            </p>
          </div>
        ) : (
          <div className="space-y-3">
            {credentials.map(credential => (
              <div
                key={credential.id}
                className="flex items-center justify-between p-4 rounded-lg border bg-card"
              >
                <div className="flex items-center gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                    <Fingerprint className="h-5 w-5 text-primary" />
                  </div>
                  <div>
                    <p className="font-medium">{credential.display_name || 'Unnamed passkey'}</p>
                    <div className="flex items-center gap-3 text-xs text-muted-foreground">
                      <span className="flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        Created {formatDate(credential.created_at)}
                      </span>
                      {credential.last_used_at && (
                        <span>Last used {formatDate(credential.last_used_at)}</span>
                      )}
                      {credential.backup_state && (
                        <span className="text-green-600 dark:text-green-500">Synced</span>
                      )}
                    </div>
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => handleDelete(credential.id)}
                  disabled={deletingId === credential.id}
                  className="text-destructive hover:text-destructive hover:bg-destructive/10"
                >
                  {deletingId === credential.id ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Trash2 className="h-4 w-4" />
                  )}
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
