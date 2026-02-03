'use client'

import { Suspense, useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { AlertCircle, Loader2, Mail, Lock, User, Eye, EyeOff, Send, CheckCircle2, Check } from 'lucide-react'
import { useLogin, useRegister, useSendMagicLink } from '@/lib/hooks/useAuth'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { PasswordStrengthMeter } from '@/components/ui/password-strength-meter'
import { PasskeyLoginButton } from '@/components/auth/passkey-login'
import { PasskeySignupButton } from '@/components/auth/passkey-signup'
import { GoogleOAuthButton } from '@/components/auth/google-oauth-button'

// Password validation constants
const MIN_PASSWORD_LENGTH = 12
const MAX_PASSWORD_LENGTH = 128

// Validation schemas
const loginSchema = z.object({
  email: z.string().email('Please enter a valid email address'),
  password: z.string().min(1, 'Password is required'),
})

const signupSchema = z.object({
  email: z.string().email('Please enter a valid email address'),
  password: z
    .string()
    .min(MIN_PASSWORD_LENGTH, `Password must be at least ${MIN_PASSWORD_LENGTH} characters`)
    .max(MAX_PASSWORD_LENGTH, `Password must be no more than ${MAX_PASSWORD_LENGTH} characters`),
  termsAccepted: z
    .boolean()
    .refine((val) => val === true, {
      message: 'You must agree to the Terms of Service and Privacy Policy',
    }),
})

type LoginFormData = z.infer<typeof loginSchema>
type SignupFormData = z.infer<typeof signupSchema>

/**
 * Safely extract error message from TanStack Form validation errors
 */
function getErrorMessage(err: unknown): string {
  if (typeof err === 'string') {
    return err
  }
  if (err && typeof err === 'object' && 'message' in err) {
    return String((err as { message: unknown }).message)
  }
  return String(err)
}

function LoginForm({ returnTo }: { returnTo: string }) {
  const router = useRouter()
  const loginMutation = useLogin()
  const sendMagicLink = useSendMagicLink()
  const { setUser } = useAuthContext()
  const [showPassword, setShowPassword] = useState(false)
  const [passkeyError, setPasskeyError] = useState<string | null>(null)
  const [magicLinkSent, setMagicLinkSent] = useState(false)
  const [magicLinkError, setMagicLinkError] = useState<string | null>(null)

  const form = useForm({
    defaultValues: {
      email: '',
      password: '',
    } as LoginFormData,
    onSubmit: async ({ value }) => {
      loginMutation.mutate(value, {
        onSuccess: data => {
          if (data.user) {
            setUser({
              id: data.user.id,
              email: data.user.email,
              first_name: data.user.first_name,
              last_name: data.user.last_name,
              email_verified: false,
            })
          }
          router.push(returnTo)
        },
      })
    },
    validators: {
      onSubmit: loginSchema,
    },
  })

  const handleSendMagicLink = async () => {
    const email = form.getFieldValue('email')
    if (!email || !z.string().email().safeParse(email).success) {
      setMagicLinkError('Please enter a valid email address first')
      return
    }

    setMagicLinkError(null)
    setMagicLinkSent(false)

    sendMagicLink.mutate(
      { email },
      {
        onSuccess: data => {
          if (data.success) {
            setMagicLinkSent(true)
          } else if (data.error_code === 'EMAIL_NOT_VERIFIED') {
            setMagicLinkError(data.message)
          } else {
            // Generic success message (even if user doesn't exist - for security)
            setMagicLinkSent(true)
          }
        },
        onError: () => {
          setMagicLinkError('Failed to send magic link. Please try again.')
        },
      }
    )
  }

  return (
    <form
      onSubmit={e => {
        e.preventDefault()
        e.stopPropagation()
        form.handleSubmit()
      }}
      className="space-y-4"
    >
      {(loginMutation.error || passkeyError) && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{loginMutation.error?.message || passkeyError}</AlertDescription>
        </Alert>
      )}

      {/* OAuth and Passkey login options */}
      <div className="space-y-3">
        <GoogleOAuthButton className="w-full" variant="login" />
        <PasskeyLoginButton
          onError={setPasskeyError}
          className="w-full"
        />
        <div className="relative">
          <div className="absolute inset-0 flex items-center">
            <span className="w-full border-t" />
          </div>
          <div className="relative flex justify-center text-xs uppercase">
            <span className="bg-card px-2 text-muted-foreground">Or continue with email</span>
          </div>
        </div>
      </div>

      <form.Field name="email">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={field.name}>Email</Label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={field.name}
                name={field.name}
                type="email"
                placeholder="you@example.com"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => {
                  field.handleChange(e.target.value)
                  // Reset magic link states when email changes
                  setMagicLinkSent(false)
                  setMagicLinkError(null)
                }}
                className="pl-10"
                aria-invalid={field.state.meta.errors.length > 0}
              />
            </div>
            {field.state.meta.errors.length > 0 && (
              <p className="text-sm text-destructive">
                {field.state.meta.errors.map(getErrorMessage).join(', ')}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Field name="password">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={field.name}>Password</Label>
            <div className="relative">
              <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={field.name}
                name={field.name}
                type={showPassword ? 'text' : 'password'}
                placeholder="Enter your password"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => field.handleChange(e.target.value)}
                className="pl-10 pr-10"
                aria-invalid={field.state.meta.errors.length > 0}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded-sm"
                aria-label={showPassword ? 'Hide password' : 'Show password'}
                aria-pressed={showPassword}
              >
                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {field.state.meta.errors.length > 0 && (
              <p className="text-sm text-destructive">
                {field.state.meta.errors.map(getErrorMessage).join(', ')}
              </p>
            )}
            <div className="flex justify-end">
              <button
                type="button"
                onClick={handleSendMagicLink}
                className="text-xs text-muted-foreground hover:text-primary underline-offset-4 hover:underline"
                disabled={sendMagicLink.isPending}
              >
                Forgot password?
              </button>
            </div>
          </div>
        )}
      </form.Field>

      <form.Subscribe selector={state => [state.canSubmit, state.isSubmitting]}>
        {([canSubmit, isSubmitting]) => (
          <Button
            type="submit"
            className="w-full"
            disabled={!canSubmit || isSubmitting || loginMutation.isPending}
          >
            {isSubmitting || loginMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Signing in...
              </>
            ) : (
              'Sign in'
            )}
          </Button>
        )}
      </form.Subscribe>

      {/* Magic link option */}
      <div className="relative">
        <div className="absolute inset-0 flex items-center">
          <span className="w-full border-t" />
        </div>
        <div className="relative flex justify-center text-xs">
          <span className="bg-card px-2 text-muted-foreground">or</span>
        </div>
      </div>

      {magicLinkSent ? (
        <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
          <CheckCircle2 className="h-4 w-4 shrink-0" />
          <span>Check your email for a sign-in link. It expires in 15 minutes.</span>
        </div>
      ) : (
        <>
          {magicLinkError && (
            <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{magicLinkError}</span>
            </div>
          )}
          <Button
            type="button"
            variant="outline"
            className="w-full"
            onClick={handleSendMagicLink}
            disabled={sendMagicLink.isPending}
          >
            {sendMagicLink.isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Sending...
              </>
            ) : (
              <>
                <Send className="h-4 w-4" />
                Email me a sign-in link
              </>
            )}
          </Button>
        </>
      )}
    </form>
  )
}

function SignupForm({ returnTo }: { returnTo: string }) {
  const router = useRouter()
  const registerMutation = useRegister()
  const { setUser } = useAuthContext()
  const [showPassword, setShowPassword] = useState(false)
  const [passwordValue, setPasswordValue] = useState('')
  const [passkeyError, setPasskeyError] = useState<string | null>(null)

  const form = useForm({
    defaultValues: {
      email: '',
      password: '',
      termsAccepted: false,
    } as SignupFormData,
    onSubmit: async ({ value }) => {
      registerMutation.mutate(
        { email: value.email, password: value.password },
        {
          onSuccess: data => {
            if (data.user) {
              setUser({
                id: data.user.id,
                email: data.user.email,
                first_name: data.user.first_name,
                last_name: data.user.last_name,
                email_verified: false,
              })
              router.push(returnTo)
            }
          },
        }
      )
    },
    validators: {
      onChange: signupSchema,
      onSubmit: signupSchema,
    },
  })

  return (
    <form
      onSubmit={e => {
        e.preventDefault()
        e.stopPropagation()
        form.handleSubmit()
      }}
      className="space-y-4"
    >
      {(registerMutation.error || passkeyError) && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{registerMutation.error?.message || passkeyError}</AlertDescription>
        </Alert>
      )}

      {/* OAuth and Passkey signup options */}
      <div className="space-y-3">
        <GoogleOAuthButton className="w-full" variant="signup" />
        <PasskeySignupButton
          onError={setPasskeyError}
          className="w-full"
        />
        <div className="relative">
          <div className="absolute inset-0 flex items-center">
            <span className="w-full border-t" />
          </div>
          <div className="relative flex justify-center text-xs uppercase">
            <span className="bg-card px-2 text-muted-foreground">Or continue with email</span>
          </div>
        </div>
      </div>

      <form.Field name="email">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={`signup-${field.name}`}>Email</Label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={`signup-${field.name}`}
                name={field.name}
                type="email"
                placeholder="you@example.com"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => field.handleChange(e.target.value)}
                className="pl-10"
                aria-invalid={field.state.meta.errors.length > 0}
              />
            </div>
            {field.state.meta.errors.length > 0 && (
              <p className="text-sm text-destructive">
                {field.state.meta.errors.map(getErrorMessage).join(', ')}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Field name="password">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={`signup-${field.name}`}>Password</Label>
            <div className="relative">
              <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={`signup-${field.name}`}
                name={field.name}
                type={showPassword ? 'text' : 'password'}
                placeholder="Create a password"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => {
                  field.handleChange(e.target.value)
                  setPasswordValue(e.target.value)
                }}
                className="pl-10 pr-10"
                aria-invalid={field.state.meta.errors.length > 0}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded-sm"
                aria-label={showPassword ? 'Hide password' : 'Show password'}
                aria-pressed={showPassword}
              >
                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {/* Password strength meter with requirements */}
            <PasswordStrengthMeter password={passwordValue} showRequirements={true} />
          </div>
        )}
      </form.Field>

      <form.Field name="termsAccepted">
        {field => (
          <div className="space-y-2">
            <div className="flex items-start space-x-3">
              <Checkbox
                id="terms"
                checked={field.state.value}
                onCheckedChange={(checked) => field.handleChange(checked === true)}
                aria-invalid={field.state.meta.errors.length > 0}
                className="mt-0.5"
              />
              <Label
                htmlFor="terms"
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
            {field.state.meta.errors.length > 0 && (
              <p className="text-sm text-destructive">
                {field.state.meta.errors.map(getErrorMessage).join(', ')}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Subscribe selector={state => [state.canSubmit, state.isSubmitting]}>
        {([canSubmit, isSubmitting]) => (
          <Button
            type="submit"
            className="w-full"
            disabled={!canSubmit || isSubmitting || registerMutation.isPending}
          >
            {isSubmitting || registerMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Creating account...
              </>
            ) : (
              'Create account'
            )}
          </Button>
        )}
      </form.Subscribe>
    </form>
  )
}

function AuthPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading } = useAuthContext()

  // Get error from URL query params (e.g., OAuth errors)
  const urlError = searchParams.get('error')

  // Get returnTo from URL query params (for redirecting after login)
  const returnTo = searchParams.get('returnTo') || '/'

  // Redirect if already authenticated
  useEffect(() => {
    if (isAuthenticated && !isLoading) {
      router.push(returnTo)
    }
  }, [isAuthenticated, isLoading, router, returnTo])

  // Show loading state while checking auth
  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  // Don't render the form if authenticated (will redirect)
  if (isAuthenticated) {
    return null
  }

  return (
    <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background px-4 py-12">
      <div className="w-full max-w-md">
        {/* Header */}
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <User className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">
            Welcome to Psychic Homily
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Join the Arizona music community
          </p>
        </div>

        {/* OAuth/URL Error Display */}
        {urlError && (
          <Alert variant="destructive" className="mb-6">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>
              {decodeURIComponent(urlError)}
            </AlertDescription>
          </Alert>
        )}

        {/* Auth Card with Tabs */}
        <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="pb-4">
            <Tabs defaultValue="login" className="w-full">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="login">Sign in</TabsTrigger>
                <TabsTrigger value="signup">Create account</TabsTrigger>
              </TabsList>

              <TabsContent value="login" className="mt-6">
                <CardTitle className="text-lg">
                  Sign in to your account
                </CardTitle>
                <CardDescription className="mt-1">
                  Enter your email and password to continue
                </CardDescription>
                <div className="mt-4">
                  <LoginForm returnTo={returnTo} />
                </div>
              </TabsContent>

              <TabsContent value="signup" className="mt-6">
                <CardTitle className="text-lg">Create an account</CardTitle>
                <CardDescription className="mt-1">
                  Sign up to submit shows and join the community
                </CardDescription>
                <div className="mt-4">
                  <SignupForm returnTo={returnTo} />
                </div>
              </TabsContent>
            </Tabs>
          </CardHeader>
        </Card>

        {/* Footer */}
        <p className="mt-6 text-center text-xs text-muted-foreground">
          By signing in, you agree to our{' '}
          <Link href="/terms" className="underline underline-offset-4 hover:text-primary">
            Terms of Service
          </Link>{' '}
          and{' '}
          <Link href="/privacy" className="underline underline-offset-4 hover:text-primary">
            Privacy Policy
          </Link>
          .
        </p>

        {/* Recovery link */}
        <p className="mt-4 text-center text-xs text-muted-foreground">
          Deleted your account?{' '}
          <Link href="/auth/recover" className="underline underline-offset-4 hover:text-primary">
            Recover it here
          </Link>
        </p>
      </div>
    </div>
  )
}

export default function AuthPage() {
  return (
    <Suspense fallback={
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    }>
      <AuthPageContent />
    </Suspense>
  )
}
