import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { Activity, Box, Columns3, SearchX } from 'lucide-react'
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Badge,
  Button,
  Checkbox,
  CheckboxOption,
  Chip,
  ControlLabel,
  EmptyState,
  Field,
  HeaderDivider,
  Heading,
  Input,
  PageHeader,
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTrigger,
  Readout,
  ReadoutStrip,
  SearchField,
  SectionHeader,
  SegmentedControl,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Sidebar,
  SidebarItem,
  Skeleton,
  Spinner,
  StatusDot,
  StatusIcon,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableScroll,
  TableSortButton,
  Text,
} from '@/ui'

// Development-only catalog of every token role, primitive and pattern. Main
// loads it only when import.meta.env.DEV is true, so production omits it.

type Theme = 'light' | 'dark'

const themes: { value: Theme; label: string }[] = [
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
]

const surfaces = [
  ['surface', 'bg-surface'],
  ['surface-raised', 'bg-surface-raised'],
  ['surface-overlay', 'bg-surface-overlay'],
  ['surface-field', 'bg-surface-field'],
  ['surface-rail', 'bg-surface-rail'],
  ['surface-heading', 'bg-surface-heading'],
  ['surface-hover', 'bg-surface-hover'],
  ['selected', 'bg-selected'],
  ['status-danger-surface', 'bg-status-danger-surface'],
] as const

const inks = [
  ['ink', 'bg-ink'],
  ['ink-muted', 'bg-ink-muted'],
  ['ink-subtle', 'bg-ink-subtle'],
  ['ink-hover', 'bg-ink-hover'],
  ['line', 'bg-line'],
  ['line-strong', 'bg-line-strong'],
  ['action', 'bg-action'],
  ['action-ink', 'bg-action-ink'],
  ['selected-ink', 'bg-selected-ink'],
  ['focus', 'bg-focus'],
  ['status-success', 'bg-status-success'],
  ['status-warning', 'bg-status-warning'],
  ['status-danger', 'bg-status-danger'],
] as const

const charts = [
  ['chart-1', 'bg-chart-1'],
  ['chart-2', 'bg-chart-2'],
  ['chart-3', 'bg-chart-3'],
  ['chart-4', 'bg-chart-4'],
  ['chart-5', 'bg-chart-5'],
  ['chart-6', 'bg-chart-6'],
  ['chart-7', 'bg-chart-7'],
  ['chart-8', 'bg-chart-8'],
  ['chart-9', 'bg-chart-9'],
  ['chart-10', 'bg-chart-10'],
  ['chart-11', 'bg-chart-11'],
  ['chart-12', 'bg-chart-12'],
] as const

const typeScale = [
  ['text-page', 'text-page leading-tight font-semibold tracking-tight'],
  ['text-readout', 'text-readout leading-tight font-semibold'],
  ['text-title', 'text-title font-semibold'],
  ['text-body', 'text-body'],
  ['text-label', 'text-label'],
] as const

const buttonVariants = [
  'default',
  'outline',
  'secondary',
  'ghost',
  'quiet',
  'link',
  'destructive',
] as const

const buttonSizes = ['compact', 'default', 'lg'] as const

function noop() {}

function initialTheme(): Theme {
  return new URLSearchParams(window.location.search).get('theme') === 'dark' ? 'dark' : 'light'
}

export function DesignCatalog() {
  const [theme, setTheme] = useState<Theme>(initialTheme)
  const [period, setPeriod] = useState('monthly')
  const [metric, setMetric] = useState('total')
  useEffect(() => {
    document.documentElement.dataset.theme = theme
  }, [theme])

  return (
    <div className="grid min-h-svh grid-cols-(--shell-columns) max-nav:grid-cols-1">
      <Sidebar label="Catalog navigation" navLabel="Catalog sections">
        {['tokens', 'controls', 'feedback', 'data', 'patterns'].map((id) => (
          <SidebarItem key={id}>
            <a href={`#${id}`}>{id}</a>
          </SidebarItem>
        ))}
      </Sidebar>
      <div className="min-w-0">
        <PageHeader title="/ Design catalog">
          <SegmentedControl
            label="Theme"
            options={themes}
            value={theme}
            onValueChange={setTheme}
            size="compact"
          />
        </PageHeader>
        <main className="mx-auto grid max-w-content gap-12 px-(--page-gutter) py-6">
          <CatalogSection id="tokens" title="Tokens">
            <Swatches title="Surfaces" swatches={surfaces} />
            <Swatches title="Ink, lines and status" swatches={inks} />
            <Swatches title="Chart series" swatches={charts} />
            <Example title="Type scale">
              <div className="grid gap-2">
                {typeScale.map(([name, className]) => (
                  <div key={name} className="flex items-baseline gap-4">
                    <code className="w-28 shrink-0 font-mono text-label text-ink-subtle">
                      {name}
                    </code>
                    <span className={className}>258K tokens</span>
                  </div>
                ))}
              </div>
            </Example>
            <Example title="Radius and control heights">
              <div className="flex flex-wrap items-end gap-4">
                <div className="size-12 rounded-sm border border-line-strong" title="radius-sm" />
                <div className="size-12 rounded-md border border-line-strong" title="radius-md" />
                <div className="size-12 rounded-lg border border-line-strong" title="radius-lg" />
                <div className="h-(--control-height-compact) w-12 bg-surface-hover" />
                <div className="h-(--control-height) w-12 bg-surface-hover" />
                <div className="h-(--control-height-lg) w-12 bg-surface-hover" />
              </div>
            </Example>
          </CatalogSection>

          <CatalogSection id="controls" title="Controls">
            <Example title="Button variants by size">
              <div className="grid gap-3">
                {buttonSizes.map((size) => (
                  <div key={size} className="flex flex-wrap items-center gap-3">
                    {buttonVariants.map((variant) => (
                      <Button key={variant} variant={variant} size={size}>
                        {variant}
                      </Button>
                    ))}
                  </div>
                ))}
              </div>
            </Example>
            <Example title="Button states">
              <div className="flex flex-wrap items-center gap-3">
                <Button variant="outline" size="compact" selected>
                  Selected
                </Button>
                <Button variant="outline" size="compact" disabled>
                  Disabled
                </Button>
                <Button variant="outline" size="icon-compact" aria-label="Columns">
                  <Columns3 aria-hidden="true" />
                </Button>
                <Button variant="outline" size="icon" aria-label="Activity">
                  <Activity aria-hidden="true" />
                </Button>
              </div>
            </Example>
            <Example title="Segmented controls">
              <div className="flex flex-wrap items-center gap-6">
                <SegmentedControl
                  label="Chart metric"
                  options={[
                    { value: 'total', label: 'Total' },
                    { value: 'input', label: 'Input' },
                    { value: 'output', label: 'Output' },
                  ]}
                  value={metric}
                  onValueChange={setMetric}
                  size="compact"
                />
                <SegmentedControl
                  label="Period"
                  variant="outline"
                  options={[
                    { value: 'daily', label: 'Daily' },
                    { value: 'monthly', label: 'Monthly' },
                  ]}
                  value={period}
                  onValueChange={setPeriod}
                />
              </div>
            </Example>
            <Example title="Fields">
              <div className="grid max-w-form gap-4">
                <Field label="Key name" description="Shown in the key list.">
                  {(control) => <Input {...control} placeholder="Work laptop" />}
                </Field>
                <Field label="New password" error="Use 15–128 characters.">
                  {(control) => <Input {...control} type="password" defaultValue="short" />}
                </Field>
                <SearchField aria-label="Search model" placeholder="Search model" />
                <ControlLabel label="Sort">
                  <Select defaultValue="total">
                    <SelectTrigger size="compact" aria-label="Sort by">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="total">Total</SelectItem>
                      <SelectItem value="date">Date</SelectItem>
                    </SelectContent>
                  </Select>
                </ControlLabel>
                <div className="flex items-center gap-2">
                  <Checkbox id="catalog-checkbox" defaultChecked aria-label="Checked" />
                  <CheckboxOption id="catalog-option" label="model-a" />
                </div>
              </div>
            </Example>
            <Example title="Chips and popover">
              <div className="flex flex-wrap items-center gap-2">
                <Chip
                  dimension="Model"
                  value="model-a"
                  removeLabel="Remove model model-a"
                  onRemove={noop}
                />
                <Chip
                  dimension="Harness"
                  value="pi"
                  removeLabel="Remove harness pi"
                  onRemove={noop}
                />
                <Popover>
                  <PopoverTrigger asChild>
                    <Button variant="outline" size="compact">
                      Open popover
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent aria-label="Catalog popover">
                    <PopoverHeader
                      title="Model"
                      action={
                        <Button variant="link" size="compact">
                          Clear
                        </Button>
                      }
                    />
                    <CheckboxOption id="catalog-popover-a" label="model-a" />
                    <CheckboxOption id="catalog-popover-b" label="model-b" />
                  </PopoverContent>
                </Popover>
              </div>
            </Example>
          </CatalogSection>

          <CatalogSection id="feedback" title="Feedback">
            <Example title="Alerts">
              <div className="grid gap-3">
                <Alert>
                  <AlertTitle>Processing usage</AlertTitle>
                  <AlertDescription>
                    Saved usage stays visible while processing runs.
                  </AlertDescription>
                </Alert>
                <Alert variant="destructive">
                  <AlertTitle>Connection unavailable</AlertTitle>
                  <AlertDescription>Retry once the server is reachable.</AlertDescription>
                  <Button variant="outline" size="compact">
                    Retry
                  </Button>
                </Alert>
              </div>
            </Example>
            <Example title="Status">
              <div className="flex flex-wrap items-center gap-4 text-label">
                <StatusIcon state="ready" />
                <StatusIcon state="loading" />
                <StatusIcon state="attention" />
                <Spinner />
                <StatusDot tone="success" />
                <StatusDot tone="pending" />
                <StatusDot tone="danger" />
                <Badge>60</Badge>
                <Badge variant="default">New</Badge>
                <Badge variant="outline">Outline</Badge>
                <Badge variant="destructive">Error</Badge>
              </div>
            </Example>
            <Example title="Skeleton and empty state">
              <Skeleton className="h-(--skeleton-readout-height)" />
              <EmptyState
                icon={<SearchX aria-hidden="true" />}
                title="No usage matches these filters"
                description="Try a wider date range or clear your filters."
                actions={<Button variant="outline">Clear filters</Button>}
              />
            </Example>
          </CatalogSection>

          <CatalogSection id="data" title="Data">
            <ReadoutStrip aria-label="Catalog readouts">
              <Readout
                primary
                label="Total tokens"
                value="258K"
                valueLabel="Total tokens: 258,000"
                detail="Included in your usage total"
              />
              <Readout label="Input" value="60K" valueLabel="Input: 60,000" detail="Uncached" />
              <Readout label="Output" value="12K" valueLabel="Output: 12,000" />
              <Readout label="Cache read" value="180K" valueLabel="Cache read: 180,000" />
              <Readout label="Sessions" value="60" valueLabel="Sessions: 60" />
            </ReadoutStrip>
            <SectionHeader
              title="Models breakdown"
              count="2"
              actions={
                <Button variant="outline" size="compact">
                  <Columns3 aria-hidden="true" />
                  Columns
                </Button>
              }
            />
            <TableScroll aria-label="Catalog table">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead aria-sort="ascending">
                      <TableSortButton>Model</TableSortButton>
                    </TableHead>
                    <TableHead numeric>
                      <TableSortButton>Total</TableSortButton>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow>
                    <TableCell>model-a</TableCell>
                    <TableCell numeric>129K</TableCell>
                  </TableRow>
                  <TableRow>
                    <TableCell>model-b</TableCell>
                    <TableCell numeric>129K</TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </TableScroll>
          </CatalogSection>

          <CatalogSection id="patterns" title="Typography and patterns">
            <Example title="Headings and text">
              <div className="grid gap-2">
                <Heading as="h1" variant="page">
                  Page heading
                </Heading>
                <Heading variant="section">Section heading</Heading>
                <Heading as="h3" variant="sub">
                  Sub heading
                </Heading>
                <Heading as="h3" variant="view">
                  View heading
                </Heading>
                <Text>Body text for explanations and table cells.</Text>
                <Text variant="secondary">Secondary text for supporting copy.</Text>
                <Text variant="muted">Muted label text for metadata.</Text>
                <Text variant="danger">Danger text for field errors.</Text>
              </div>
            </Example>
            <Example title="Page header">
              <div className="border border-line">
                <PageHeader title="/ Models">
                  <span className="text-label font-semibold">controlbox</span>
                  <HeaderDivider />
                  <Button variant="quiet" size="icon-compact" aria-label="Models">
                    <Box aria-hidden="true" />
                  </Button>
                </PageHeader>
              </div>
            </Example>
          </CatalogSection>
        </main>
      </div>
    </div>
  )
}

function CatalogSection({
  id,
  title,
  children,
}: {
  id: string
  title: string
  children: ReactNode
}) {
  return (
    <section id={id} aria-labelledby={`${id}-heading`} className="grid gap-6">
      <Heading id={`${id}-heading`} as="h2" variant="page">
        {title}
      </Heading>
      {children}
    </section>
  )
}

function Example({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="grid gap-3">
      <Heading as="h3" variant="sub">
        {title}
      </Heading>
      {children}
    </div>
  )
}

function Swatches({
  title,
  swatches,
}: {
  title: string
  swatches: readonly (readonly [string, string])[]
}) {
  return (
    <Example title={title}>
      <ul className="grid grid-cols-(--readout-columns-narrow) gap-3 wide:grid-cols-4">
        {swatches.map(([name, className]) => (
          <li key={name} className="flex items-center gap-2 text-label">
            <span className={`size-8 shrink-0 rounded-md border border-line ${className}`} />
            <code className="font-mono">--{name}</code>
          </li>
        ))}
      </ul>
    </Example>
  )
}
