'use client'

import { useState } from 'react'
import { browserSupportsWebAuthn } from '@simplewebauthn/browser'
import { Fingerprint, Trash2, Loader2, Shield, Clock } from 'lucide-react'
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
            onSuccess={() => {
              setActionError(null)
              credentialsQuery.refetch()
            }}
            onError={err => setActionError(err)}
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
            {credentials.map(credential => {
              const isDeleting =
                deletePasskey.isPending && deletePasskey.variables === credential.id
              return (
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
                          <span className="text-success-foreground">Synced</span>
                        )}
                      </div>
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleDelete(credential.id)}
                    disabled={isDeleting}
                    className="text-destructive hover:text-destructive hover:bg-destructive/10"
                  >
                    {isDeleting ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <Trash2 className="h-4 w-4" />
                    )}
                  </Button>
                </div>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
