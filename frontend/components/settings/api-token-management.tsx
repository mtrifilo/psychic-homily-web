'use client'

import { useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import { useAPITokens, useCreateAPIToken, useRevokeAPIToken, type APIToken } from '@/lib/hooks/useAuth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogClose,
} from '@/components/ui/dialog'
import {
  Key,
  Plus,
  Trash2,
  Copy,
  Check,
  AlertCircle,
  Loader2,
  Clock,
  CheckCircle2,
} from 'lucide-react'

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function formatDateTime(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

function TokenRow({ token, onRevoke, isRevoking }: {
  token: APIToken
  onRevoke: (id: number) => void
  isRevoking: boolean
}) {
  const [isRevokeDialogOpen, setIsRevokeDialogOpen] = useState(false)
  const expiresAt = new Date(token.expires_at)
  const isExpiringSoon = !token.is_expired && expiresAt.getTime() - Date.now() < 7 * 24 * 60 * 60 * 1000 // 7 days

  const handleRevoke = () => {
    onRevoke(token.id)
    setIsRevokeDialogOpen(false)
  }

  return (
    <div className="flex items-center justify-between rounded-lg border border-border/50 bg-muted/30 p-4">
      <div className="flex-1 space-y-1">
        <div className="flex items-center gap-2">
          <span className="font-medium text-sm">
            {token.description || 'Unnamed token'}
          </span>
          {token.is_expired ? (
            <Badge variant="secondary" className="bg-destructive/10 text-destructive border-0 text-xs">
              Expired
            </Badge>
          ) : isExpiringSoon ? (
            <Badge variant="secondary" className="bg-amber-500/10 text-amber-600 dark:text-amber-400 border-0 text-xs">
              Expiring soon
            </Badge>
          ) : (
            <Badge variant="secondary" className="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-0 text-xs">
              Active
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
          <span>Created: {formatDate(token.created_at)}</span>
          <span>Expires: {formatDate(token.expires_at)}</span>
          {token.last_used_at && (
            <span>Last used: {formatDateTime(token.last_used_at)}</span>
          )}
        </div>
      </div>
      <Dialog open={isRevokeDialogOpen} onOpenChange={setIsRevokeDialogOpen}>
        <DialogTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 text-muted-foreground hover:text-destructive"
            disabled={isRevoking}
          >
            {isRevoking ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Trash2 className="h-4 w-4" />
            )}
          </Button>
        </DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Revoke API Token</DialogTitle>
            <DialogDescription>
              Are you sure you want to revoke this token? Any applications using this token will immediately lose access.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setIsRevokeDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleRevoke}
              disabled={isRevoking}
            >
              {isRevoking ? (
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              ) : null}
              Revoke Token
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export function APITokenManagement() {
  const { data: tokensData, isLoading, error } = useAPITokens()
  const createToken = useCreateAPIToken()
  const revokeToken = useRevokeAPIToken()

  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false)
  const [description, setDescription] = useState('')
  const [expirationDays, setExpirationDays] = useState('90')
  const [newToken, setNewToken] = useState<string | null>(null)
  const [tokenCopied, setTokenCopied] = useState(false)

  const handleCreateToken = async () => {
    try {
      const response = await createToken.mutateAsync({
        description: description || undefined,
        expiration_days: parseInt(expirationDays) || 90,
      })
      setNewToken(response.token)
      setDescription('')
      setExpirationDays('90')
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'api-tokens', error_type: 'create_failed' },
      })
      console.error('Failed to create token:', error)
    }
  }

  const handleCopyToken = async () => {
    if (newToken) {
      await navigator.clipboard.writeText(newToken)
      setTokenCopied(true)
      setTimeout(() => setTokenCopied(false), 2000)
    }
  }

  const handleCloseCreateDialog = () => {
    setIsCreateDialogOpen(false)
    setNewToken(null)
    setTokenCopied(false)
  }

  const handleRevoke = async (tokenId: number) => {
    try {
      await revokeToken.mutateAsync(tokenId)
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'api-tokens', error_type: 'revoke_failed' },
        extra: { tokenId },
      })
      console.error('Failed to revoke token:', error)
    }
  }

  const tokens = tokensData?.tokens || []
  const activeTokens = tokens.filter(t => !t.is_expired)

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Key className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-lg">API Tokens</CardTitle>
          </div>
          <Dialog open={isCreateDialogOpen} onOpenChange={(open) => {
            if (!open) handleCloseCreateDialog()
            else setIsCreateDialogOpen(true)
          }}>
            <DialogTrigger asChild>
              <Button size="sm" className="gap-2">
                <Plus className="h-4 w-4" />
                Create Token
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>
                  {newToken ? 'Token Created' : 'Create API Token'}
                </DialogTitle>
                <DialogDescription>
                  {newToken
                    ? 'Copy your token now. It will not be shown again.'
                    : 'Create a long-lived API token for the discovery app or other tools.'
                  }
                </DialogDescription>
              </DialogHeader>

              {newToken ? (
                <div className="space-y-4 py-4">
                  <div className="flex items-center gap-2 rounded-lg border bg-muted/50 p-3">
                    <code className="flex-1 text-xs font-mono break-all">
                      {newToken}
                    </code>
                    <Button
                      variant="outline"
                      size="icon"
                      onClick={handleCopyToken}
                      className="shrink-0"
                    >
                      {tokenCopied ? (
                        <Check className="h-4 w-4 text-emerald-500" />
                      ) : (
                        <Copy className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                  <div className="flex items-center gap-2 rounded-md bg-amber-500/10 p-3 text-sm text-amber-600 dark:text-amber-400">
                    <AlertCircle className="h-4 w-4 shrink-0" />
                    <span>Store this token securely. It cannot be retrieved after you close this dialog.</span>
                  </div>
                  {tokenCopied && (
                    <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
                      <CheckCircle2 className="h-4 w-4" />
                      <span>Token copied to clipboard!</span>
                    </div>
                  )}
                </div>
              ) : (
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <Label htmlFor="description">Description (optional)</Label>
                    <Input
                      id="description"
                      placeholder="e.g., Discovery App on Mike's laptop"
                      value={description}
                      onChange={(e) => setDescription(e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="expiration">Expiration (days)</Label>
                    <Input
                      id="expiration"
                      type="number"
                      min="1"
                      max="365"
                      value={expirationDays}
                      onChange={(e) => setExpirationDays(e.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">
                      Token will expire after this many days (max 365)
                    </p>
                  </div>
                </div>
              )}

              <DialogFooter>
                {newToken ? (
                  <Button onClick={handleCloseCreateDialog}>Done</Button>
                ) : (
                  <>
                    <Button variant="outline" onClick={() => setIsCreateDialogOpen(false)}>
                      Cancel
                    </Button>
                    <Button
                      onClick={handleCreateToken}
                      disabled={createToken.isPending}
                      className="gap-2"
                    >
                      {createToken.isPending ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Key className="h-4 w-4" />
                      )}
                      Create Token
                    </Button>
                  </>
                )}
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
        <CardDescription>
          Long-lived tokens for the local discovery app and other admin tools
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {isLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : error ? (
            <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4" />
              <span>Failed to load tokens. Please try again.</span>
            </div>
          ) : tokens.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border/50 bg-muted/30 p-6 text-center">
              <Key className="mx-auto h-8 w-8 text-muted-foreground/50" />
              <p className="mt-2 text-sm text-muted-foreground">
                No API tokens yet
              </p>
              <p className="text-xs text-muted-foreground">
                Create a token to use with the local discovery app
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {tokens.map((token) => (
                <TokenRow
                  key={token.id}
                  token={token}
                  onRevoke={handleRevoke}
                  isRevoking={revokeToken.isPending}
                />
              ))}
            </div>
          )}

          {/* Info about token usage */}
          <div className="rounded-lg border border-border/50 bg-muted/30 p-4">
            <div className="flex items-start gap-3">
              <Clock className="h-5 w-5 text-muted-foreground mt-0.5" />
              <div className="space-y-1">
                <p className="text-sm font-medium text-foreground">Token Usage</p>
                <p className="text-xs text-muted-foreground">
                  Use these tokens with the local discovery app. Set the token in the app&apos;s settings,
                  and it will be used for all API requests. Tokens are valid for up to 365 days.
                </p>
              </div>
            </div>
          </div>

          {createToken.isError && (
            <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4" />
              <span>
                {createToken.error?.message || 'Failed to create token. Please try again.'}
              </span>
            </div>
          )}

          {revokeToken.isError && (
            <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4" />
              <span>
                {revokeToken.error?.message || 'Failed to revoke token. Please try again.'}
              </span>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
