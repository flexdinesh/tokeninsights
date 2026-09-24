import '@testing-library/jest-dom/vitest'
import { beforeEach, vi } from 'vitest'

// JSDOM does not implement media queries; browser tests cover responsive changes.
beforeEach(() => {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: true,
    media: query,
    onchange: null,
    addEventListener: vi.fn<MediaQueryList['addEventListener']>(),
    removeEventListener: vi.fn<MediaQueryList['removeEventListener']>(),
    addListener: vi.fn<MediaQueryList['addListener']>(),
    removeListener: vi.fn<MediaQueryList['removeListener']>(),
    dispatchEvent: vi.fn<MediaQueryList['dispatchEvent']>(),
  }))
})
