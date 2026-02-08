'use client'

import { useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import { startRegistration, browserSupportsWebAuthn } from '@simplewebauthn/browser'
import { Fingerprint, Loader2, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
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
} from '@/components/ui/dialog'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

interface PasskeyRegisterButtonProps {
  onSuccess?: () => void
  onError?: (error: string) => void
  className?: string
}

export function PasskeyRegisterButton({ onSuccess, onError, className }: PasskeyRegisterButtonProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [displayName, setDisplayName] = useState('')

  const supportsWebAuthn = browserSupportsWebAuthn()

  const handleRegister = async () => {
    if (!supportsWebAuthn) {
      onError?.('Your browser does not support passkeys')
      return
    }

    setIsLoading(true)

    try {
      // Step 1: Begin registration
      const beginResponse = await fetch(`${API_BASE_URL}/auth/passkey/register/begin`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ display_name: displayName }),
      })

      const beginData = await beginResponse.json()

      if (!beginData.success) {
        throw new Error(beginData.message || 'Failed to start passkey registration')
      }

      // Step 2: Perform WebAuthn registration
      const credential = await startRegistration({
        optionsJSON: beginData.options,
      })

      // Step 3: Finish registration
      const finishResponse = await fetch(`${API_BASE_URL}/auth/passkey/register/finish`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          challenge_id: beginData.challenge_id,
          display_name: displayName || 'My Passkey',
          response: credential,
        }),
      })

      const finishData = await finishResponse.json()

      if (!finishData.success) {
        throw new Error(finishData.message || 'Failed to register passkey')
      }

      // Success
      setIsOpen(false)
      setDisplayName('')
      onSuccess?.()
    } catch (error) {
      if (error instanceof Error) {
        if (error.name === 'NotAllowedError') {
          // User cancelled
          return
        }
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'passkey-auth', error_type: 'registration_failed' },
        })
        onError?.(error.message)
      } else {
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'passkey-auth', error_type: 'registration_failed' },
        })
        onError?.('An unexpected error occurred')
      }
    } finally {
      setIsLoading(false)
    }
  }

  if (!supportsWebAuthn) {
    return null
  }

  return (
    <Dialog open={isOpen} onOpenChange={setIsOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" className={className}>
          <Plus className="h-4 w-4" />
          Add a passkey
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Fingerprint className="h-5 w-5" />
            Add a passkey
          </DialogTitle>
          <DialogDescription>
            Passkeys let you sign in securely using your device&apos;s biometrics (Face ID, Touch ID,
            Windows Hello) or a security key.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="passkey-name">Passkey name</Label>
            <Input
              id="passkey-name"
              placeholder="e.g., My MacBook, Work laptop"
              value={displayName}
              onChange={e => setDisplayName(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Give this passkey a name to identify it later
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setIsOpen(false)} disabled={isLoading}>
            Cancel
          </Button>
          <Button onClick={handleRegister} disabled={isLoading}>
            {isLoading ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Registering...
              </>
            ) : (
              <>
                <Fingerprint className="h-4 w-4" />
                Register passkey
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
