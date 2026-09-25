import { afterEach, expect, it } from 'vitest'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { UnknownLocation } from './ResultsTable'

afterEach(cleanup)

it('reveals all recorded directories on request and identifies missing directory data', async () => {
  const user = userEvent.setup()
  render(
    <UnknownLocation
      directories={['~/work/one', '~/work/two', '~/work/three', '~/work/four']}
      hasUnknownDirectory
    />,
  )

  expect(screen.queryByRole('list', { name: 'Recorded directories' })).not.toBeInTheDocument()
  expect(screen.queryByText('Some usage has no recorded directory')).not.toBeInTheDocument()
  const toggle = screen.getByRole('button', { name: 'Show directories' })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')

  await user.click(toggle)
  const list = screen.getByRole('list', { name: 'Recorded directories' })
  expect(within(list).getAllByRole('listitem')).toHaveLength(4)
  expect(screen.getByText('Some usage has no recorded directory')).toBeVisible()
  const collapse = screen.getByRole('button', { name: 'Hide directories' })
  expect(collapse).toHaveAttribute('aria-expanded', 'true')
  await user.click(collapse)
  expect(screen.queryByRole('list', { name: 'Recorded directories' })).not.toBeInTheDocument()
  expect(screen.queryByText('Some usage has no recorded directory')).not.toBeInTheDocument()
})

it('does not show a reveal control when no directory was recorded', () => {
  render(<UnknownLocation directories={[]} hasUnknownDirectory />)
  expect(screen.getByText('unknown')).toBeVisible()
  expect(screen.getByText('No recorded directory')).toBeVisible()
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})
