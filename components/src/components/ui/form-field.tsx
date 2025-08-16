import { type AnyFieldApi } from '@tanstack/react-form'
import { Input } from '@/components/ui/input'

interface FormFieldProps {
    field: AnyFieldApi
    label: string
    type?: 'text' | 'date' | 'time' | 'textarea'
    placeholder?: string
    onEnterPress?: () => void
}

export function FieldInfo({ field }: Readonly<{ field: AnyFieldApi }>) {
    return (
        <>
            {field.state.meta.isTouched && !field.state.meta.isValid ? (
                <em>{field.state.meta.errors.map((err) => err.message).join(',')}</em>
            ) : null}
            {field.state.meta.isValidating ? 'Validating...' : null}
        </>
    )
}

export function FormField({ field, label, type = 'text', placeholder, onEnterPress }: Readonly<FormFieldProps>) {
    return (
        <div className="flex flex-col space-y-2">
            <label htmlFor={field.name} className="text-sm font-medium text-gray-700 dark:text-gray-300">
                {label}
            </label>
            {type === 'textarea' ? (
                <textarea
                    className="w-full min-h-[100px] px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent dark:bg-gray-700 dark:border-gray-600 dark:text-white"
                    id={field.name}
                    name={field.name}
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => {
                        field.handleChange(e.target.value)
                    }}
                    placeholder={placeholder}
                />
            ) : (
                <Input
                    type={type}
                    className="w-full"
                    id={field.name}
                    name={field.name}
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => {
                        field.handleChange(e.target.value)
                    }}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                            e.preventDefault()
                            onEnterPress?.()
                        }
                    }}
                    placeholder={placeholder}
                />
            )}
            <FieldInfo field={field} />
        </div>
    )
}
