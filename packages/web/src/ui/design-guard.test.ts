// @vitest-environment node
import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// Design system guard. Tailwind silently ignores unknown classes and CSS
// silently ignores undefined custom properties, so these checks keep features
// on the token contract and primitives as the only owners of appearance.

const srcDirectory = fileURLToPath(new URL('..', import.meta.url))
const tokenDirectory = 'ui/tokens/'
const paletteFile = 'ui/tokens/palette.css'

const sources = readdirSync(srcDirectory, { recursive: true, encoding: 'utf8' })
  .map((path) => path.replaceAll('\\', '/'))
  .filter((path) => /\.(ts|tsx|css)$/.test(path))
  .filter((path) => !/\.test\.tsx?$/.test(path) && path !== 'test-setup.ts')
  .filter((path) => !path.startsWith('generated/') && !path.startsWith('mocks/'))
  .map((path) => ({ path, text: readFileSync(`${srcDirectory}${path}`, 'utf8') }))

function matches(pattern: RegExp, files = sources) {
  return files.flatMap(({ path, text }) =>
    [...text.matchAll(pattern)].map((match) => `${path}: ${match[0]}`),
  )
}

// Third-party and engine-owned custom properties.
const externalProperty = /^--(tw|radix|lightningcss)-/

function definedProperties() {
  const defined = new Set<string>()
  for (const { path, text } of sources) {
    if (!path.startsWith(tokenDirectory) && path !== 'ui/utilities.css') continue
    for (const match of text.matchAll(/(--[\w-]+)\s*:/g)) {
      const property = match[1]
      if (property) defined.add(property)
    }
  }
  return defined
}

// Utility part of a class: everything after the last variant separator that
// sits outside brackets, so arbitrary variants such as [&_svg]: stay allowed.
function utilityOf(className: string) {
  let depth = 0
  let start = 0
  for (let index = 0; index < className.length; index++) {
    const character = className[index]
    if (character === '[' || character === '(') depth++
    else if (character === ']' || character === ')') depth--
    else if (character === ':' && depth === 0) start = index + 1
  }
  return className.slice(start).replace(/^!/, '')
}

const classStrings = sources
  .filter(({ path }) => path.endsWith('.tsx') || path.endsWith('.ts'))
  .flatMap(({ path, text }) =>
    [...text.matchAll(/(['"`])((?:(?!\1)[^\\\n]|\\.)*)\1/g)].flatMap((match) =>
      (match[2] ?? '').split(/\s+/).map((className) => ({ path, className })),
    ),
  )

const paletteNames =
  'slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose'
const colorUtilities =
  'text|bg|border|border-[xytrblse]|ring|ring-offset|fill|stroke|outline|from|via|to|divide|shadow|accent|caret|decoration|placeholder'
// Tailwind default scales removed by the namespace resets in theme.css.
const resetScaleUtility = new RegExp(
  [
    `^-?(${colorUtilities})-((${paletteNames})-\\d{2,3}|white|black)(/\\d+)?$`,
    '^text-(xs|sm|base|lg|\\d?xl)$',
    '^rounded(-[trblse]{1,2})?-(xs|xl|\\dxl)$',
    '^(shadow|inset-shadow|drop-shadow|text-shadow)-(2xs|xs|sm|md|lg|xl|2xl)$',
    '^font-(thin|extralight|light|extrabold|black)$',
    '^leading-(none|snug|relaxed|loose)$',
    '^tracking-(tighter|wide|wider|widest|normal)$',
    '^(max-w|min-w|w)-(3xs|2xs|xs|sm|md|lg|xl|\\dxl)$',
  ].join('|'),
)
const resetBreakpointVariant = /(^|:)(max-)?(sm|md|lg|xl|2xl|@(3xs|2xs|xs|sm|md|lg|xl|\dxl)):/

describe('design guard', () => {
  it('keeps raw color values in the palette', () => {
    const colors = /#[0-9a-f]{3,8}\b|\b(oklch|oklab|rgba?|hsla?|lab|lch|color)\(/gi
    expect(
      matches(
        colors,
        sources.filter(({ path }) => path !== paletteFile),
      ),
    ).toEqual([])
  })

  it('references only defined custom properties', () => {
    const defined = definedProperties()
    const undefinedReferences = sources.flatMap(({ path, text }) =>
      [...text.matchAll(/(?:var\(|-\()\s*(--[\w-]+)/g)]
        .map((match) => match[1] ?? '')
        .filter((property) => !externalProperty.test(property) && !defined.has(property))
        .map((property) => `${path}: ${property}`),
    )
    expect(undefinedReferences).toEqual([])
  })

  it('uses tokens instead of arbitrary Tailwind values', () => {
    const arbitrary = classStrings
      .filter(({ className }) => {
        const utility = utilityOf(className)
        return /^-?[a-z][\w-]*-\[.+\]$/.test(utility) || /^\[[a-z-]+:.+\]$/.test(utility)
      })
      .map(({ path, className }) => `${path}: ${className}`)
    expect(arbitrary).toEqual([])
  })

  it('uses token scales instead of reset Tailwind defaults', () => {
    const defaults = classStrings
      .filter(
        ({ className }) =>
          resetScaleUtility.test(utilityOf(className)) || resetBreakpointVariant.test(className),
      )
      .map(({ path, className }) => `${path}: ${className}`)
    expect(defaults).toEqual([])
  })

  it('keeps Radix, cva and class merging inside src/ui', () => {
    const imports = /from ['"](@radix-ui\/[\w-]+|class-variance-authority|tailwind-merge|clsx)['"]/g
    expect(
      matches(
        imports,
        sources.filter(({ path }) => !path.startsWith('ui/')),
      ),
    ).toEqual([])
  })

  it('loads the design catalog only behind the development flag', () => {
    const references = sources
      .filter(({ path }) => !path.startsWith('design/'))
      .filter(({ text }) => /design\/DesignCatalog|_design/.test(text))
      .map(({ path }) => path)
    expect(references).toEqual(['main.tsx'])
    const main = sources.find(({ path }) => path === 'main.tsx')?.text ?? ''
    expect(main).not.toMatch(/^import .*design\//m)
    expect(main).toMatch(
      /if \(import\.meta\.env\.DEV && window\.location\.pathname === '\/_design'\) \{\n\s+const \{ DesignCatalog \} = await import\('\.\/design\/DesignCatalog'\)/,
    )
  })
})
