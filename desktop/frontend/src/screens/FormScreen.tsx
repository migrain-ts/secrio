import type { ReactNode } from 'react'
import { Card, CardContent, CardHeader } from '@/components/ui/card'

/** A standalone form outside the app shell (unlock, first run): one card,
 *  max ~27.5rem wide (ui notes: product-page widths), title then one line. */
export function FormScreen({ title, description, children }: { title: string; description?: ReactNode; children: ReactNode }) {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center bg-background px-4 py-12">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Secrio</p>
      <Card className="w-full max-w-[27.5rem]">
        <CardHeader className="gap-1.5 space-y-0">
          <h1 className="text-xl font-semibold tracking-tight text-foreground">{title}</h1>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </CardHeader>
        <CardContent>{children}</CardContent>
      </Card>
    </main>
  )
}
