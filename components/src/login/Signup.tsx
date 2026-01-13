import {
    Dialog,
    DialogTrigger,
    DialogContent,
    DialogTitle,
    DialogFooter,
    DialogHeader,
    DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { DialogClose } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useRegister } from '@/lib/hooks/useAuth'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useState } from 'react'
import { Eye, EyeOff } from 'lucide-react'

interface SignupForm {
    email: string
    password: string
    confirm_password: string
}

interface SignupValidationError {
    password_mismatch: boolean
    password_length: boolean
    email_required: boolean
    password_required: boolean
    confirm_password_required: boolean
}

interface ServerError {
    is_error: boolean
    failure_count: number
    failure_reason: string
    status: number | null
    details: any | null
}

// Helper function to create error state
const createErrorState = (overrides: Partial<SignupValidationError> = {}): SignupValidationError => ({
    password_mismatch: false,
    password_length: false,
    email_required: false,
    password_required: false,
    confirm_password_required: false,
    ...overrides,
})

// Validation function that checks all conditions
const validateForm = (email: string, password: string, confirmPassword: string): SignupValidationError => {
    return createErrorState({
        email_required: email === '',
        password_required: password === '',
        confirm_password_required: confirmPassword === '',
        password_length: password !== '' && password.length < 8,
        password_mismatch: password !== '' && confirmPassword !== '' && password !== confirmPassword,
    })
}

function Signup() {
    const [signupValidationError, setSignupValidationError] = useState<SignupValidationError | null>(null)
    const [serverError, setServerError] = useState<ServerError | null>(null)
    const [showPassword, setShowPassword] = useState(false)
    const [showConfirmPassword, setShowConfirmPassword] = useState(false)

    const registerMutation = useRegister()
    const { setUser, clearError } = useAuthContext()

    const handleSignup = (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault()
        e.stopPropagation()
        setSignupValidationError(null)
        setServerError(null)
        clearError()
        const form = e.currentTarget

        // Type the form data
        const formData: SignupForm = {
            email: form.email.value,
            password: form.password.value,
            confirm_password: form.confirm_password.value,
        }

        const validationErrors = validateForm(formData.email, formData.password, formData.confirm_password)
        const hasErrors = Object.values(validationErrors).some((error) => error)

        if (hasErrors) {
            setSignupValidationError(validationErrors)
            return
        }

        const { email, password } = formData

        registerMutation.mutate(
            {
                email,
                password,
            },
            {
                onSuccess: (data) => {
                    if (data.user) {
                        // Convert the API user format to AuthContext User format
                        const user = {
                            id: data.user.id,
                            email: data.user.email,
                            first_name: data.user.first_name,
                            last_name: data.user.last_name,
                            email_verified: false, // Default value, update when profile endpoint is available
                            created_at: new Date().toISOString(), // Default value
                            updated_at: new Date().toISOString(), // Default value
                        }
                        setUser(user)
                    }
                },
                onError: (error) => {
                    // Try to parse the detailed error from the API
                    let errorDetails = {
                        is_error: true,
                        failure_count: registerMutation.failureCount,
                        failure_reason: error?.toString() || 'Unknown error',
                        status: null,
                        details: null,
                    }

                    try {
                        // Parse the detailed error if it's a JSON string
                        const parsedError = JSON.parse(error.message)
                        errorDetails = {
                            is_error: true,
                            failure_count: registerMutation.failureCount,
                            failure_reason: parsedError.message || error.message,
                            status: parsedError.status,
                            details: parsedError.details,
                        }
                    } catch (e) {
                        // If parsing fails, use the original error
                        console.log('Could not parse error details:', e)
                    }

                    setServerError(errorDetails)
                },
            }
        )
    }

    return (
        <div className="mb-4">
            <Dialog>
                <DialogTrigger asChild>
                    <Button>Signup</Button>
                </DialogTrigger>
                <DialogContent className="sm:max-w-[425px]">
                    <form onSubmit={(e) => handleSignup(e)}>
                        <DialogHeader>
                            <DialogTitle>Signup</DialogTitle>
                            <DialogDescription className="mb-4">Signup for an account.</DialogDescription>
                        </DialogHeader>

                        {signupValidationError && (
                            <Alert variant="destructive" className="mb-4">
                                <AlertDescription>
                                    <div className="space-y-1">
                                        <div className="font-medium">Please fix the following errors to signup:</div>
                                        {signupValidationError.email_required && <div>• Email is required</div>}
                                        {signupValidationError.password_required && <div>• Password is required</div>}
                                        {signupValidationError.confirm_password_required && (
                                            <div>• Confirm password is required</div>
                                        )}
                                        {signupValidationError.password_length && (
                                            <div>• Password must be at least 8 characters</div>
                                        )}
                                        {signupValidationError.password_mismatch && <div>• Passwords do not match</div>}
                                    </div>
                                </AlertDescription>
                            </Alert>
                        )}

                        {serverError?.is_error && (
                            <Alert variant="destructive" className="mb-4">
                                <AlertDescription>
                                    <div className="space-y-1">
                                        <div className="font-medium">An unexpected error occurred.</div>
                                        <div>{serverError.failure_reason}</div>
                                    </div>
                                </AlertDescription>
                            </Alert>
                        )}
                        <div className="grid gap-4">
                            <div className="grid gap-3">
                                <Label htmlFor="name-1">Email</Label>
                                <Input id="email" name="email" placeholder="email" />
                            </div>
                            <div className="grid gap-3 mb-4">
                                <Label htmlFor="password-1">Password</Label>
                                <div className="relative">
                                    <Input
                                        id="password"
                                        name="password"
                                        type={showPassword ? 'text' : 'password'}
                                        placeholder="password"
                                        className="pr-10"
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
                            </div>
                            <div className="grid gap-3 mb-4">
                                <Label htmlFor="confirm-password-1">Confirm Password</Label>
                                <div className="relative">
                                    <Input
                                        id="confirm_password"
                                        name="confirm_password"
                                        type={showConfirmPassword ? 'text' : 'password'}
                                        placeholder="confirm password"
                                        className="pr-10"
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
                            </div>
                        </div>
                        <DialogFooter>
                            <DialogClose asChild>
                                <Button variant="outline">Cancel</Button>
                            </DialogClose>
                            <Button type="submit" disabled={registerMutation.isPending}>
                                {registerMutation.isPending ? 'Signing up...' : 'Signup'}
                            </Button>
                        </DialogFooter>
                    </form>
                </DialogContent>
            </Dialog>
        </div>
    )
}

export default Signup
