'use client'

import { useState } from 'react'
import { useOAuthAccounts, useUnlinkOAuthAccount } from '@/lib/hooks/useAuth'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Loader2,
  Link2,
  Link2Off,
} from 'lucide-react'

// Use direct backend URL for OAuth (not the Next.js proxy)
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

// Google "G" logo SVG component
function GoogleIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
        fill="#4285F4"
      />
      <path
        d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
        fill="#34A853"
      />
      <path
        d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"
        fill="#FBBC05"
      />
      <path
        d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
        fill="#EA4335"
      />
    </svg>
  )
}

export function OAuthAccounts() {
  const { data, isLoading, error } = useOAuthAccounts()
  const unlinkMutation = useUnlinkOAuthAccount()
  const [unlinkProvider, setUnlinkProvider] = useState<string | null>(null)

  const handleConnectGoogle = () => {
    window.location.href = `${API_BASE_URL}/auth/login/google`
  }

  const handleUnlink = async () => {
    if (!unlinkProvider) return

    try {
      await unlinkMutation.mutateAsync(unlinkProvider)
      setUnlinkProvider(null)
    } catch {
      // Error is handled by the mutation
    }
  }

  const googleAccount = data?.accounts?.find(acc => acc.provider === 'google')

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Link2 className="h-5 w-5 text-muted-foreground" />
          <CardTitle className="text-lg">Connected Accounts</CardTitle>
        </div>
        <CardDescription>
          Manage your connected sign-in methods
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {/* Google Account */}
          <div className="flex items-center justify-between rounded-lg border border-border/50 bg-muted/30 p-4">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-background">
                <GoogleIcon className="h-5 w-5" />
              </div>
              <div>
                {googleAccount ? (
                  <>
                    <p className="text-sm font-medium">
                      {googleAccount.email || googleAccount.name || 'Google Account'}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      Connected {new Date(googleAccount.connected_at).toLocaleDateString()}
                    </p>
                  </>
                ) : (
                  <>
                    <p className="text-sm font-medium">Google</p>
                    <p className="text-xs text-muted-foreground">
                      Not connected
                    </p>
                  </>
                )}
              </div>
            </div>

            {isLoading ? (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            ) : googleAccount ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setUnlinkProvider('google')}
                disabled={unlinkMutation.isPending}
              >
                {unlinkMutation.isPending && unlinkProvider === 'google' ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <>
                    <Link2Off className="h-4 w-4" />
                    Disconnect
                  </>
                )}
              </Button>
            ) : (
              <Button
                variant="outline"
                size="sm"
                onClick={handleConnectGoogle}
              >
                <Link2 className="h-4 w-4" />
                Connect
              </Button>
            )}
          </div>

          {/* Error display */}
          {error && (
            <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>Failed to load connected accounts</span>
            </div>
          )}

          {/* Unlink error */}
          {unlinkMutation.isError && (
            <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{unlinkMutation.error?.message || 'Failed to disconnect account'}</span>
            </div>
          )}

          {/* Success message */}
          {unlinkMutation.isSuccess && (
            <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
              <CheckCircle2 className="h-4 w-4 shrink-0" />
              <span>Account disconnected successfully</span>
            </div>
          )}
        </div>

        {/* Unlink confirmation dialog */}
        <Dialog open={!!unlinkProvider} onOpenChange={(open) => !open && setUnlinkProvider(null)}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2 text-destructive">
                <AlertTriangle className="h-5 w-5" />
                Disconnect Google Account?
              </DialogTitle>
              <DialogDescription>
                You will no longer be able to sign in with this Google account.
                Make sure you have another way to access your account (password or passkey).
              </DialogDescription>
            </DialogHeader>
            <DialogFooter className="gap-2 sm:gap-0">
              <Button
                variant="outline"
                onClick={() => setUnlinkProvider(null)}
                disabled={unlinkMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleUnlink}
                disabled={unlinkMutation.isPending}
              >
                {unlinkMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  'Disconnect'
                )}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  )
}
