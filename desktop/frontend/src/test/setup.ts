import '@testing-library/jest-dom/vitest'
import { cleanup, configure } from '@testing-library/react'
import { afterEach } from 'vitest'

// The full-app screen tests are heavy in jsdom; under a parallel run the
// default 1 s for findBy/waitFor is too tight and makes them flaky.
configure({ asyncUtilTimeout: 4000 })

afterEach(() => cleanup())
