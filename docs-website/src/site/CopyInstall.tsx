'use client'

import { useState } from 'react'

const RESET_MS = 1500

export function CopyInstall(props: CopyInstall.Props) {
  const [label, setLabel] = useState('Copy')

  async function copy() {
    try {
      await navigator.clipboard.writeText(props.command)
      setLabel('Copied')
      setTimeout(() => setLabel('Copy'), RESET_MS)
    } catch {
      setLabel('Select and copy')
    }
  }

  return (
    <button className="ti-copy" type="button" onClick={copy}>
      <svg
        className="ti-icon"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.75"
        aria-hidden="true"
      >
        <rect x="9" y="9" width="11" height="11" />
        <path d="M5 15H4V4h11v1" />
      </svg>
      <span aria-live="polite">{label}</span>
    </button>
  )
}

export declare namespace CopyInstall {
  type Props = { command: string }
}
