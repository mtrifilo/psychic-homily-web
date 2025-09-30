import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import {
    Dialog,
    DialogTrigger,
    DialogContent,
    DialogTitle,
    DialogFooter,
    DialogHeader,
    DialogDescription,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { DialogClose } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useLogin } from '@/lib/hooks/useAuth'
import { useAuthContext } from '@/lib/context/AuthContext'
import { XCircle } from 'lucide-react'

// Zod schema for validation
const loginSchema = z.object({
    email: z.string().email('Please enter a valid email address'),
    password: z.string().min(1, 'Password is required'),
})

// Form data interface
interface LoginFormData {
    email: string
    password: string
}

function Login() {
    const loginMutation = useLogin()
    const { user, clearError } = useAuthContext()

    const form = useForm({
        defaultValues: {
            email: '',
            password: '',
        } as LoginFormData,
        onSubmit: async ({ value }) => {
            loginMutation.mutate(value, {
                onSuccess: (data) => {
                    console.log('Data from login mutation:', data)
                    console.log('User from auth context:', user)
                },
                onError: (error) => {
                    // Error handling is done by the mutation
                },
            })
        },
        validators: {
            onSubmit: loginSchema,
        },
    })

    return (
        <div className="mb-4">
            <Dialog
                onOpenChange={(open) => {
                    if (!open) {
                        // Clear form and errors when dialog closes
                        form.reset()
                        clearError()
                    }
                }}
            >
                <DialogTrigger asChild>
                    <Button>Login</Button>
                </DialogTrigger>
                <DialogContent className="sm:max-w-[425px]">
                    <form
                        onSubmit={(e) => {
                            e.preventDefault()
                            e.stopPropagation()
                            form.handleSubmit()
                        }}
                    >
                        <DialogHeader>
                            <DialogTitle>Login</DialogTitle>
                            <DialogDescription className="mb-4">Login to your account to continue.</DialogDescription>
                        </DialogHeader>

                        {/* Server errors from mutation */}
                        {loginMutation.error && (
                            <Alert variant="destructive" className="mb-4">
                                <XCircle className="h-4 w-4" />
                                <AlertDescription>{loginMutation.error.message}</AlertDescription>
                            </Alert>
                        )}

                        <div className="grid gap-4">
                            <form.Field
                                name="email"
                                children={(field) => (
                                    <div className="grid gap-3">
                                        <Label htmlFor={field.name}>Email</Label>
                                        <Input
                                            id={field.name}
                                            name={field.name}
                                            type="email"
                                            placeholder="Enter your email"
                                            value={field.state.value}
                                            onBlur={field.handleBlur}
                                            onChange={(e) => field.handleChange(e.target.value)}
                                        />
                                        {field.state.meta.errors.length > 0 && (
                                            <Alert variant="destructive">
                                                <XCircle className="h-4 w-4" />
                                                <AlertDescription>
                                                    {field.state.meta.errors
                                                        .map((err) => err?.message || String(err))
                                                        .join(', ')}
                                                </AlertDescription>
                                            </Alert>
                                        )}
                                    </div>
                                )}
                            />

                            <form.Field
                                name="password"
                                children={(field) => (
                                    <div className="grid gap-3 mb-4">
                                        <Label htmlFor={field.name}>Password</Label>
                                        <Input
                                            id={field.name}
                                            name={field.name}
                                            type="password"
                                            placeholder="Enter your password"
                                            value={field.state.value}
                                            onBlur={field.handleBlur}
                                            onChange={(e) => field.handleChange(e.target.value)}
                                        />
                                        {field.state.meta.errors.length > 0 && (
                                            <Alert variant="destructive">
                                                <XCircle className="h-4 w-4" />
                                                <AlertDescription>
                                                    {field.state.meta.errors
                                                        .map((err) => err?.message || String(err))
                                                        .join(', ')}
                                                </AlertDescription>
                                            </Alert>
                                        )}
                                    </div>
                                )}
                            />
                        </div>

                        <DialogFooter className="gap-3">
                            <DialogClose asChild>
                                <Button variant="outline" className="flex-1">
                                    Cancel
                                </Button>
                            </DialogClose>
                            <form.Subscribe
                                selector={(state) => [state.canSubmit, state.isSubmitting]}
                                children={([canSubmit, isSubmitting]) => (
                                    <Button
                                        type="submit"
                                        disabled={!canSubmit || isSubmitting || loginMutation.isPending}
                                        className="flex-1"
                                    >
                                        {isSubmitting || loginMutation.isPending ? (
                                            <div className="flex items-center gap-2">
                                                <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                                                Logging in...
                                            </div>
                                        ) : (
                                            'Login'
                                        )}
                                    </Button>
                                )}
                            />
                        </DialogFooter>
                    </form>
                </DialogContent>
            </Dialog>
        </div>
    )
}

export default Login
