import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { Button } from './components/ui/button'
import { FormTextarea } from './components/ui/form-textarea'

const formSchema = z.object({
    prompt: z.string().min(1, 'Prompt is required'),
})

export const AiForm = () => {
    const form = useForm({
        defaultValues: { prompt: '' },
        onSubmit: async ({ value }) => {
            // send value.prompt to your backend here
            console.log('submitted:', value)
        },
        validators: {
            onSubmit: formSchema,
        },
    })

    return (
        <div className="flex flex-col items-start justify-center w-md">
            <form
                className="w-full space-y-4"
                onSubmit={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    form.handleSubmit()
                }}
            >
                <form.Field name="prompt">
                    {(field) => (
                        <FormTextarea
                            field={field}
                            label="Prompt"
                            placeholder="Enter your prompt for the AI..."
                            onEnterPress={form.handleSubmit}
                        />
                    )}
                </form.Field>
                <form.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting]}>
                    {([canSubmit, isSubmitting]) => (
                        <Button type="submit" disabled={!canSubmit || isSubmitting}>
                            {isSubmitting ? 'Submitting...' : 'Submit'}
                        </Button>
                    )}
                </form.Subscribe>
            </form>
        </div>
    )
}
