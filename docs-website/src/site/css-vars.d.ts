// Custom properties set inline (`--ss-accent`, `--ti-line-d`) type-check without assertions.
export type CustomProperty = `--${string}`

declare module 'react' {
  interface CSSProperties {
    [property: CustomProperty]: string | number | undefined
  }
}
