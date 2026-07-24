'use client'

import { useState } from 'react'
import { browserSupportsWebAuthn } from '@simplewebauthn/browser'
import { Trash2, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  PasskeyRegisterButton,
  usePasskeyCredentials,
  useDeletePasskey,
} from '@/features/auth'
import { isAuthError } from '@/lib/errors'

export function PasskeyManagement() {
  const supportsWebAuthn = browserSupportsWebAuthn()

  // Credential list comes from the shared query cache (PSY-1102) — no more
  // raw fetch-in-effect or hand-rolled loading state. Gated on WebAuthn
  // support so unsupported browsers never issue the request.
  const credentialsQuery = usePasskeyCredentials(supportsWebAuthn)
  const deletePasskey = useDeletePasskey()

  // Action errors (delete failures + register-button callbacks) are surfaced
  // separately from the load error so a failed delete doesn't read as "failed
  // to load passkeys" and vice versa.
  const [actionError, setActionError] = useState<string | null>(null)

  const credentials = credentialsQuery.data ?? []
  const isLoading = credentialsQuery.isLoading
  // A server-side `{ success: false }` surfaces as an AuthError carrying the
  // backend message (e.g. "Unauthorized"); any other failure (network throw,
  // parse error) stays generic — matching the pre-migration split.
  const loadError = credentialsQuery.isError
    ? isAuthError(credentialsQuery.error)
      ? credentialsQuery.error.message
      : 'Failed to load passkeys'
    : null
  const error = actionError ?? loadError

  const handleDelete = (credentialId: number) => {
    if (!confirm('Are you sure you want to remove this passkey?')) {
      return
    }

    setActionError(null)
    deletePasskey.mutate(credentialId, {
      onSuccess: () => setActionError(null),
      onError: err => {
        setActionError(err instanceof Error ? err.message : 'Failed to delete passkey')
      },
    })
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

        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1 space-y-3">
            {isLoading ? (
              <div className="flex items-center py-1">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : credentials.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No passkeys registered yet. Add a passkey for faster, more secure sign-in.
              </p>
            ) : (
              credentials.map(credential => {
                const lastUsed = formatDate(credential.last_used_at)
                const isDeleting =
                  deletePasskey.isPending && deletePasskey.variables === credential.id
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
                      disabled={isDeleting}
                      className="shrink-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
                      aria-label={`Remove ${credential.display_name || 'passkey'}`}
                    >
                      {isDeleting ? (
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
            onSuccess={() => {
              setActionError(null)
              credentialsQuery.refetch()
            }}
            onError={err => setActionError(err)}
          />
        </div>
      </CardContent>
    </Card>
  )
}
