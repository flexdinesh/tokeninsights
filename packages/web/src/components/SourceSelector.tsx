import { useId, useState } from 'react'
import { Check, ChevronDown, LoaderCircle, Plus, Server, Trash2 } from 'lucide-react'
import { getInstance } from '../api'
import { useSources } from '../source-context'
import { createSource, normalizeBaseUrl } from '../sources'
import type { Bootstrap } from '../contracts'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover'

const requiredCapabilities: Bootstrap['capabilities'] = ['usage', 'facets', 'sync']

export function SourceSelector({ unavailable }: { unavailable: boolean }) {
  const { sources, active, add, select, remove } = useSources()
  const [open, setOpen] = useState(false)
  const [input, setInput] = useState('')
  const [error, setError] = useState('')
  const [validating, setValidating] = useState(false)
  const errorId = useId()
  const localUrl = normalizeBaseUrl(window.location.origin)

  const submit = async () => {
    setError('')
    setValidating(true)
    try {
      const baseUrl = normalizeBaseUrl(input)
      const instance = await getInstance(baseUrl)
      const missing = requiredCapabilities.filter(
        (capability) => !instance.capabilities.includes(capability),
      )
      if (missing.length > 0) {
        throw new Error(`Source lacks required capabilities: ${missing.join(', ')}`)
      }
      const source = createSource(baseUrl, instance)
      add(source)
      select(source.baseUrl)
      setInput('')
      setOpen(false)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Couldn’t connect to source')
    } finally {
      setValidating(false)
    }
  }

  return (
    <Popover
      open={open}
      onOpenChange={(value) => {
        setOpen(value)
        if (!value) setError('')
      }}
    >
      <PopoverTrigger asChild>
        <Button variant="outline" className="source-trigger" aria-label="Choose data source">
          <span className={`status-dot ${unavailable ? 'unavailable' : ''}`} />
          <span className="source-trigger-copy">
            <strong>{active.hostname}</strong>
            <small>{active.baseUrl}</small>
          </span>
          <ChevronDown size="1em" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="source-popover" align="end" aria-label="Data sources">
        <div className="popover-heading">
          <strong>Data sources</strong>
          <span className="muted">{sources.length}</span>
        </div>
        <div className="source-list" role="list">
          {sources.map((source) => {
            const selected = source.baseUrl === active.baseUrl
            const local = source.baseUrl === localUrl
            return (
              <div className="source-option" role="listitem" key={source.baseUrl}>
                <Button
                  variant="ghost"
                  className="source-select"
                  aria-label={`Select ${source.hostname} source`}
                  aria-current={selected ? 'true' : undefined}
                  onClick={() => {
                    select(source.baseUrl)
                    setOpen(false)
                  }}
                >
                  {selected ? <Check size="1em" /> : <Server size="1em" />}
                  <span>
                    <strong>{source.hostname}</strong>
                    <small>{source.baseUrl}</small>
                  </span>
                </Button>
                {!local && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="source-remove"
                    aria-label={`Remove ${source.hostname} source`}
                    title={`Remove ${source.hostname}`}
                    onClick={() => remove(source.baseUrl)}
                  >
                    <Trash2 size="1em" />
                  </Button>
                )}
              </div>
            )
          })}
        </div>
        <form
          className="source-add"
          onSubmit={(event) => {
            event.preventDefault()
            void submit()
          }}
        >
          <label htmlFor={`${errorId}-url`}>Add source</label>
          <div className="source-add-row">
            <Input
              id={`${errorId}-url`}
              type="text"
              inputMode="url"
              autoCapitalize="none"
              autoCorrect="off"
              placeholder="host:port or https://host"
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? errorId : undefined}
              value={input}
              onChange={(event) => setInput(event.target.value)}
            />
            <Button disabled={validating || input.trim() === ''}>
              {validating ? <LoaderCircle className="spin" size="1em" /> : <Plus size="1em" />}
              Add
            </Button>
          </div>
          {error && (
            <p id={errorId} className="error-text" role="alert">
              {error}
            </p>
          )}
          <p className="hint">Source must expose a compatible TokenInsights API.</p>
        </form>
      </PopoverContent>
    </Popover>
  )
}
