'use client'

import { useState } from 'react'
import { ShieldAlert, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

// Use direct backend URL for OAuth (browser redirect, not AJAX)
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

interface BackupAuthPromptProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onComplete: () => void
}

export function BackupAuthPrompt({ open, onOpenChange, onComplete }: BackupAuthPromptProps) {
  const [isConnectingGoogle, setIsConnectingGoogle] = useState(false)

  const handleConnectGoogle = () => {
    setIsConnectingGoogle(true)
    // Redirect to Google OAuth - after connecting, they'll be redirected back
    // The OAuth callback will redirect to home page
    window.location.href = `${API_BASE_URL}/auth/login/google`
  }

  const handleSkip = () => {
    onOpenChange(false)
    onComplete()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md" onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldAlert className="h-5 w-5 text-amber-500" />
            Add a backup sign-in method
          </DialogTitle>
          <DialogDescription>
            Your account is secured with a passkey on this device. Adding a backup method ensures you can still access your account if you lose this device.
          </DialogDescription>
        </DialogHeader>

        <div className="py-4">
          {/* Google option */}
          <Button
            variant="outline"
            className="w-full justify-start gap-3 h-auto py-3"
            onClick={handleConnectGoogle}
            disabled={isConnectingGoogle}
          >
            {isConnectingGoogle ? (
              <Loader2 className="h-5 w-5 animate-spin" />
            ) : (
              <GoogleIcon className="h-5 w-5" />
            )}
            <div className="text-left">
              <div className="font-medium">Connect Google account</div>
              <div className="text-xs text-muted-foreground">Sign in with your Google account as a backup</div>
            </div>
          </Button>
        </div>

        <div className="rounded-lg border border-amber-500/20 bg-amber-500/5 p-3">
          <p className="text-xs text-muted-foreground">
            <strong className="text-foreground">Why add a backup?</strong> If you lose access to this device, you won&apos;t be able to sign in with your passkey. A backup method lets you recover your account.
          </p>
        </div>

        <DialogFooter>
          <Button
            variant="ghost"
            onClick={handleSkip}
            className="text-muted-foreground"
          >
            I&apos;ll do this later
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
