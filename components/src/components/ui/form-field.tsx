import { type AnyFieldApi } from "@tanstack/react-form";
import { Input } from "@/components/ui/input";

interface FormFieldProps {
  field: AnyFieldApi;
  label: string;
  type?: "text" | "date" | "time";
  placeholder?: string;
  onEnterPress?: () => void;
}

export function FieldInfo({ field }: Readonly<{ field: AnyFieldApi }>) {
  return (
    <>
      {field.state.meta.isTouched && !field.state.meta.isValid ? (
        <em>{field.state.meta.errors.map((err) => err.message).join(",")}</em>
      ) : null}
      {field.state.meta.isValidating ? "Validating..." : null}
    </>
  );
}

export function FormField({
  field,
  label,
  type = "text",
  placeholder,
  onEnterPress,
}: Readonly<FormFieldProps>) {
  return (
    <div className="flex items-center">
      <label htmlFor={field.name}>{label}:</label>
      <Input
        type={type}
        className="mr-4 ml-4"
        id={field.name}
        name={field.name}
        value={field.state.value}
        onBlur={field.handleBlur}
        onChange={(e) => {
          field.handleChange(e.target.value);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            onEnterPress?.();
          }
        }}
        placeholder={placeholder}
      />
      <FieldInfo field={field} />
    </div>
  );
}
