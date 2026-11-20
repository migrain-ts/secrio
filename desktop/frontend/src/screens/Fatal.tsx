import { WarningCircleIcon } from '@/components/icons'
import { FormScreen } from '@/screens/FormScreen'

/** Production without the Go bindings: nothing can work, and nothing here calls the api. */
export function Fatal() {
  return (
    <FormScreen title="Secrio can't reach its backend">
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <WarningCircleIcon aria-hidden className="size-4 shrink-0" />
        Restart the app.
      </p>
    </FormScreen>
  )
}
