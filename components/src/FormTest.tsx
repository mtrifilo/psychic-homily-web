import { useForm, type AnyFieldApi } from "@tanstack/react-form";
import { Input } from "@/components/ui/input";
import { z } from "zod";
import { Button } from "./components/ui/button";
import { FormField, FieldInfo } from "@/components/ui/form-field";

interface Artist {
  name: string;
}

interface ArtistFieldProp {
  pushValue: (value: Artist) => void;
}

interface ShowSubmission {
  artists: Artist[];
  venue: string;
  date: string;
  time?: string;
  cost?: string;
  ages?: string;
  city: string;
  state: string;
  description: string;
}

const defaultShowSubmission: ShowSubmission = {
  artists: [{ name: "" }],
  venue: "",
  date: "",
  time: "20:00",
  cost: "",
  ages: "",
  city: "",
  state: "",
  description: "",
};

const formSchema = z.object({
  artists: z
    .array(
      z.object({
        name: z.string().min(1, "Artist name is required"),
      })
    )
    .min(1, "At least one artist is required"),
  venue: z.string().min(1, "Venue is required"),
  date: z.string().min(1, "Date is required"),
  time: z.string().optional(),
  cost: z.string().optional(),
  ages: z.string().optional(),
  city: z.string().min(1, "City is required"),
  state: z.string().min(1, "State is required"),
  description: z.string(),
});

export const FormTest = () => {
  const form = useForm({
    defaultValues: defaultShowSubmission,
    onSubmit: async ({ value }) => {
      console.log("submitted:", value);
    },
    validators: {
      onSubmit: formSchema,
    },
  });

  const handleAddArtist = (artistsField: ArtistFieldProp) => {
    artistsField.pushValue({ name: "" });
  };

  return (
    <div className="flex flex-col items-start justify-center w-md">
      <h1 className="w-full mb-4">Form test</h1>
      <form
        className="w-full space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          e.stopPropagation();
          form.handleSubmit();
        }}
      >
        <div className="w-full">
          <form.Field
            name="artists"
            mode="array"
            children={(artistsField) => (
              <div>
                {artistsField.state.value.map((_, i) => (
                  <div key={i}>
                    <form.Field
                      name={`artists[${i}].name`}
                      children={(field) => {
                        return (
                          <div
                            className={`flex items-center ${
                              artistsField.state.value.length > 1 ? "mt-4" : ""
                            }`}
                          >
                            <label htmlFor={field.name}>Artist:</label>
                            <Input
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
                                  handleAddArtist(artistsField);
                                }
                              }}
                            />
                            <FieldInfo field={field} />
                            {artistsField.state.value.length > 1 && (
                              <Button
                                type="button"
                                variant="outline"
                                onClick={() => {
                                  artistsField.removeValue(i);
                                }}
                              >
                                X
                              </Button>
                            )}
                          </div>
                        );
                      }}
                    />
                  </div>
                ))}
                <Button
                  type="button"
                  className="mt-4"
                  onClick={() => handleAddArtist(artistsField)}
                >
                  Add another artist
                </Button>
              </div>
            )}
          />
        </div>
        <form.Field name="venue">
          {(field) => (
            <FormField
              field={field}
              label="Venue"
              onEnterPress={form.handleSubmit}
            />
          )}
        </form.Field>
        <form.Field name="date">
          {(field) => (
            <FormField
              field={field}
              label="Date"
              type="date"
              onEnterPress={form.handleSubmit}
            />
          )}
        </form.Field>
        <form.Field name="time">
          {(field) => (
            <FormField
              field={field}
              label="Time"
              type="time"
              onEnterPress={form.handleSubmit}
            />
          )}
        </form.Field>
        <form.Field name="cost">
          {(field) => (
            <FormField
              field={field}
              label="Cost"
              placeholder="e.g. $20, Free"
              onEnterPress={form.handleSubmit}
            />
          )}
        </form.Field>
        <form.Field name="ages">
          {(field) => (
            <FormField
              field={field}
              label="Ages"
              placeholder="e.g. 21+, All Ages"
              onEnterPress={form.handleSubmit}
            />
          )}
        </form.Field>
        <form.Field name="city">
          {(field) => (
            <FormField
              field={field}
              label="City"
              placeholder="e.g. Phoenix"
              onEnterPress={form.handleSubmit}
            />
          )}
        </form.Field>
        <form.Field name="state">
          {(field) => (
            <FormField
              field={field}
              label="State"
              placeholder="e.g. AZ"
              onEnterPress={form.handleSubmit}
            />
          )}
        </form.Field>
        <form.Field name="description">
          {(field) => (
            <FormField
              field={field}
              label="Description"
              onEnterPress={form.handleSubmit}
            />
          )}
        </form.Field>
        <form.Subscribe
          selector={(state) => [state.canSubmit, state.isSubmitting]}
          children={([canSubmit, isSubmitting]) => (
            <Button
              type="submit"
              disabled={!canSubmit || isSubmitting}
              className="mt-4"
            >
              {isSubmitting ? "Submitting..." : "Submit"}
            </Button>
          )}
        />
      </form>
    </div>
  );
};
