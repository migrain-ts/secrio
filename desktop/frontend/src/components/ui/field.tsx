import * as React from "react"
import { cn } from "@/lib/utils"
import { Label } from "./label"

// Linear Field: Label + Input + optional description + optional error
// Light theme: label text-sm font-medium, description/error text-xs
// Error state: error message in text-destructive

interface FieldProps extends React.HTMLAttributes<HTMLDivElement> {
  label?: string
  htmlFor?: string
  description?: string
  error?: string
  required?: boolean
  optional?: boolean
  /** Secrio: announce the error (role="alert") — for errors that arrive
   *  after a wait (a server answer), not for ones shown while typing. */
  announce?: boolean
}

const Field = React.forwardRef<HTMLDivElement, FieldProps>(
  (
    {
      label,
      htmlFor,
      description,
      error,
      required,
      optional,
      announce,
      className,
      children,
      ...props
    },
    ref
  ) => {
    const hasError = Boolean(error)

    return (
      <div
        ref={ref}
        className={cn("flex flex-col gap-1.5", className)}
        {...props}
      >
        {label && (
          <div className="flex items-center gap-1.5">
            <Label
              htmlFor={htmlFor}
              className={cn(
                "text-sm font-medium leading-none text-foreground",
                hasError && "text-destructive-text"
              )}
            >
              {label}
            </Label>
            {required && (
              <span className="text-destructive-text text-sm leading-none" aria-hidden="true">
                *
              </span>
            )}
            {optional && !required && (
              <span className="text-xs text-muted-foreground">(optional)</span>
            )}
          </div>
        )}

        <div
          className={cn(
            hasError && [
              "[&_input]:border-destructive-text",
              "[&_input]:focus-visible:border-destructive-text",
              "[&_input]:focus-visible:shadow-[0_0_0_0.1875rem_color-mix(in_oklch,var(--destructive-text)_25%,transparent)]",
              "[&_textarea]:border-destructive-text",
              "[&_select]:border-destructive-text",
            ]
          )}
        >
          {children}
        </div>

        {description && !hasError && (
          <p
            id={htmlFor ? fieldDescriptionId(htmlFor) : undefined}
            className="text-xs text-muted-foreground leading-relaxed"
          >
            {description}
          </p>
        )}

        {hasError && (
          <p
            id={htmlFor ? fieldErrorId(htmlFor) : undefined}
            role={announce ? "alert" : undefined}
            className="text-xs text-destructive-text leading-relaxed flex items-center gap-1"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 16 16"
              fill="currentColor"
              className="h-3 w-3 shrink-0"
              aria-hidden="true"
            >
              <path
                fillRule="evenodd"
                d="M8 15A7 7 0 1 0 8 1a7 7 0 0 0 0 14Zm0-10a.75.75 0 0 1 .75.75v3.5a.75.75 0 0 1-1.5 0v-3.5A.75.75 0 0 1 8 5Zm0 6.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5Z"
                clipRule="evenodd"
              />
            </svg>
            {error}
          </p>
        )}
      </div>
    )
  }
)
Field.displayName = "Field"

// Ids of the description and error lines, so the control can point at them
// with aria-describedby (Secrio: errors are announced with their field).
const fieldDescriptionId = (htmlFor: string) => `${htmlFor}-description`
const fieldErrorId = (htmlFor: string) => `${htmlFor}-error`

/** aria-describedby for the control of a Field: its error, else its description. */
function fieldDescribedBy(htmlFor: string, error?: string, description?: string): string | undefined {
  if (error) return fieldErrorId(htmlFor)
  return description ? fieldDescriptionId(htmlFor) : undefined
}

export { Field, fieldDescribedBy }
export type { FieldProps }
