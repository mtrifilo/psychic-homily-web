'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { startRegistration, browserSupportsWebAuthn } from '@simplewebauthn/browser'
import { Fingerprint, Loader2, Mail, X } from 'lucide-react'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { useAuthContext } from '@/lib/context/AuthContext'
import { BackupAuthPrompt } from './backup-auth-prompt'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

interface PasskeySignupButtonProps {
  onError?: (error: string) => void
  className?: string
}

export function PasskeySignupButton({ onError, className }: PasskeySignupButtonProps) {
  const router = useRouter()
  const { setUser } = useAuthContext()
  const [isLoading, setIsLoading] = useState(false)
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const [showBackupPrompt, setShowBackupPrompt] = useState(false)
  const [email, setEmail] = useState('')
  const [emailError, setEmailError] = useState<string | null>(null)
  const [termsAccepted, setTermsAccepted] = useState(false)
  const [termsError, setTermsError] = useState<string | null>(null)

  // Check if browser supports WebAuthn
  const supportsWebAuthn = browserSupportsWebAuthn()

  const handlePasskeySignup = async () => {
    // Validate email
    const emailSchema = z.string().email('Please enter a valid email address')
    const result = emailSchema.safeParse(email)
    if (!result.success) {
      setEmailError(result.error.issues[0].message)
      return
    }
    setEmailError(null)

    // Validate terms acceptance
    if (!termsAccepted) {
      setTermsError('You must agree to the Terms of Service and Privacy Policy')
      return
    }
    setTermsError(null)

    if (!supportsWebAuthn) {
      onError?.('Your browser does not support passkeys')
      return
    }

    setIsLoading(true)

    try {
      // Step 1: Begin signup
      const beginResponse = await fetch(`${API_BASE_URL}/auth/passkey/signup/begin`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ email }),
      })

      const beginData = await beginResponse.json()

      if (!beginData.success) {
        throw new Error(beginData.message || 'Failed to start passkey signup')
      }

      // Step 2: Perform WebAuthn registration
      const credential = await startRegistration({
        optionsJSON: beginData.options,
      })

      // Step 3: Finish signup
      const finishResponse = await fetch(`${API_BASE_URL}/auth/passkey/signup/finish`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          challenge_id: beginData.challenge_id,
          response: credential,
        }),
      })

      const finishData = await finishResponse.json()

      if (!finishData.success) {
        throw new Error(finishData.message || 'Failed to complete passkey signup')
      }

      // Success - update auth context
      if (finishData.user) {
        setUser({
          id: finishData.user.id,
          email: finishData.user.email,
          first_name: finishData.user.first_name,
          last_name: finishData.user.last_name,
          email_verified: finishData.user.email_verified,
        })
      }

      // Close signup dialog and show backup prompt
      setIsDialogOpen(false)
      setShowBackupPrompt(true)
    } catch (error) {
      // Handle user cancellation gracefully
      if (error instanceof Error) {
        if (error.name === 'NotAllowedError') {
          // User cancelled the operation
          return
        }
        onError?.(error.message)
      } else {
        onError?.('An unexpected error occurred')
      }
    } finally {
      setIsLoading(false)
    }
  }

  const handleBackupComplete = () => {
    router.push('/')
  }

  // Don't render if browser doesn't support WebAuthn
  if (!supportsWebAuthn) {
    return null
  }

  return (
    <>
    <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={className}
        >
          <Fingerprint className="h-4 w-4" />
          Sign up with passkey
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Create account with passkey</DialogTitle>
          <DialogDescription>
            Enter your email address to create an account secured with your device&apos;s passkey.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="passkey-email">Email</Label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id="passkey-email"
                type="email"
                placeholder="you@example.com"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value)
                  setEmailError(null)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    handlePasskeySignup()
                  }
                }}
                className="pl-10"
                disabled={isLoading}
              />
            </div>
            {emailError && (
              <p role="alert" className="text-sm text-destructive">{emailError}</p>
            )}
          </div>
          <div className="space-y-2">
            <div className="flex items-start space-x-3">
              <Checkbox
                id="passkey-terms"
                checked={termsAccepted}
                onCheckedChange={(checked) => {
                  setTermsAccepted(checked === true)
                  setTermsError(null)
                }}
                className="mt-0.5"
              />
              <Label
                htmlFor="passkey-terms"
                className="text-sm font-normal leading-relaxed cursor-pointer"
              >
                I agree to the{' '}
                <Link
                  href="/terms"
                  target="_blank"
                  className="font-medium underline underline-offset-4 hover:text-primary"
                >
                  Terms of Service
                </Link>{' '}
                and{' '}
                <Link
                  href="/privacy"
                  target="_blank"
                  className="font-medium underline underline-offset-4 hover:text-primary"
                >
                  Privacy Policy
                </Link>
              </Label>
            </div>
            {termsError && (
              <p role="alert" className="text-sm text-destructive">{termsError}</p>
            )}
          </div>
          <Button
            type="button"
            onClick={handlePasskeySignup}
            disabled={isLoading || !email || !termsAccepted}
            className="w-full"
          >
            {isLoading ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Creating account...
              </>
            ) : (
              <>
                <Fingerprint className="h-4 w-4" />
                Continue with passkey
              </>
            )}
          </Button>
        </div>
      </DialogContent>
    </Dialog>

    <BackupAuthPrompt
      open={showBackupPrompt}
      onOpenChange={setShowBackupPrompt}
      onComplete={handleBackupComplete}
    />
    </>
  )
}

export { browserSupportsWebAuthn }
