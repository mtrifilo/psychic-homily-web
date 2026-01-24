'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { AlertCircle, Loader2, Mail, Lock, User, Eye, EyeOff } from 'lucide-react'
import { useLogin, useRegister } from '@/lib/hooks/useAuth'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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

// Password validation constants
const MIN_PASSWORD_LENGTH = 12
const MAX_PASSWORD_LENGTH = 128

// Validation schemas
const loginSchema = z.object({
  email: z.string().email('Please enter a valid email address'),
  password: z.string().min(1, 'Password is required'),
})

const signupSchema = z
  .object({
    email: z.string().email('Please enter a valid email address'),
    password: z
      .string()
      .min(MIN_PASSWORD_LENGTH, `Password must be at least ${MIN_PASSWORD_LENGTH} characters`)
      .max(MAX_PASSWORD_LENGTH, `Password must be no more than ${MAX_PASSWORD_LENGTH} characters`),
    confirmPassword: z.string().min(1, 'Please confirm your password'),
  })
  .refine(data => data.password === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
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

function LoginForm() {
  const router = useRouter()
  const loginMutation = useLogin()
  const { setUser } = useAuthContext()
  const [showPassword, setShowPassword] = useState(false)
  const [passkeyError, setPasskeyError] = useState<string | null>(null)

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
          router.push('/')
        },
      })
    },
    validators: {
      onSubmit: loginSchema,
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
      {(loginMutation.error || passkeyError) && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{loginMutation.error?.message || passkeyError}</AlertDescription>
        </Alert>
      )}

      {/* Passkey login option */}
      <div className="space-y-3">
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
    </form>
  )
}

function SignupForm() {
  const router = useRouter()
  const registerMutation = useRegister()
  const { setUser } = useAuthContext()
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)
  const [passwordValue, setPasswordValue] = useState('')
  const [confirmPasswordValue, setConfirmPasswordValue] = useState('')

  const form = useForm({
    defaultValues: {
      email: '',
      password: '',
      confirmPassword: '',
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
              router.push('/')
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

  // Check if passwords match for real-time feedback
  const passwordsMatch = passwordValue === confirmPasswordValue && confirmPasswordValue.length > 0

  return (
    <form
      onSubmit={e => {
        e.preventDefault()
        e.stopPropagation()
        form.handleSubmit()
      }}
      className="space-y-4"
    >
      {registerMutation.error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{registerMutation.error.message}</AlertDescription>
        </Alert>
      )}

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
                placeholder="At least 12 characters"
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

      <form.Field name="confirmPassword">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={`signup-${field.name}`}>Confirm Password</Label>
            <div className="relative">
              <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={`signup-${field.name}`}
                name={field.name}
                type={showConfirmPassword ? 'text' : 'password'}
                placeholder="Confirm your password"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => {
                  field.handleChange(e.target.value)
                  setConfirmPasswordValue(e.target.value)
                }}
                className="pl-10 pr-10"
                aria-invalid={field.state.meta.errors.length > 0}
              />
              <button
                type="button"
                onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded-sm"
                aria-label={showConfirmPassword ? 'Hide password' : 'Show password'}
                aria-pressed={showConfirmPassword}
              >
                {showConfirmPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {/* Show password match status */}
            {confirmPasswordValue && (
              <p className={`text-xs ${passwordsMatch ? 'text-green-600 dark:text-green-500' : 'text-muted-foreground'}`}>
                {passwordsMatch ? 'Passwords match' : 'Passwords do not match'}
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

export default function AuthPage() {
  const router = useRouter()
  const { isAuthenticated, isLoading } = useAuthContext()

  // Redirect if already authenticated
  useEffect(() => {
    if (isAuthenticated && !isLoading) {
      router.push('/')
    }
  }, [isAuthenticated, isLoading, router])

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
                  <LoginForm />
                </div>
              </TabsContent>

              <TabsContent value="signup" className="mt-6">
                <CardTitle className="text-lg">Create an account</CardTitle>
                <CardDescription className="mt-1">
                  Sign up to submit shows and join the community
                </CardDescription>
                <div className="mt-4">
                  <SignupForm />
                </div>
              </TabsContent>
            </Tabs>
          </CardHeader>
        </Card>

        {/* Footer */}
        <p className="mt-6 text-center text-xs text-muted-foreground">
          By continuing, you agree to our terms of service and privacy policy.
        </p>
      </div>
    </div>
  )
}
