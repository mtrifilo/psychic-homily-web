import { type AnyFieldApi } from '@tanstack/react-form'
import { Textarea } from '@/components/ui/textarea'
import { FieldInfo } from './form-field'

interface FormTextareaProps {
    field: AnyFieldApi
    label: string
    placeholder?: string
    onEnterPress?: () => void
}

export function FormTextarea({ field, label, placeholder, onEnterPress }: Readonly<FormTextareaProps>) {
    return (
        <div className="flex flex-col items-start w-full mb-4">
            <label htmlFor={field.name} className="mb-1">
                {label}:
            </label>
            <Textarea
                id={field.name}
                name={field.name}
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                onKeyDown={(e) => {
                    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                        e.preventDefault()
                        onEnterPress?.()
                    }
                }}
                placeholder={placeholder}
                className="w-full"
            />
            <FieldInfo field={field} />
        </div>
    )
}
