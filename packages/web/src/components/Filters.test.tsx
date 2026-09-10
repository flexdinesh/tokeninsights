import { afterEach, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { MultiSelect } from './Filters'

afterEach(cleanup)

it('keeps selected values available when other facets remove them, and supports search and clearing', async () => {
  const user = userEvent.setup()
  function Example() {
    const [selected, setSelected] = useState(['unavailable-model'])
    return <><MultiSelect label="Model" values={['model-a', 'model-b']} selected={selected} onChange={setSelected} /><output>{selected.join(',')}</output></>
  }
  render(<Example />)
  await user.click(screen.getByRole('button', { name: /Model/ }))
  expect(screen.getByRole('checkbox', { name: 'unavailable-model' })).toBeChecked()
  await user.type(screen.getByRole('textbox', { name: 'Search model' }), 'model-a')
  expect(screen.queryByRole('checkbox', { name: 'model-b' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('checkbox', { name: 'model-a' }))
  expect(screen.getByRole('status')).toHaveTextContent('unavailable-model,model-a')
  await user.click(screen.getByRole('button', { name: 'Clear' }))
  expect(screen.getByRole('status')).toBeEmptyDOMElement()
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
})
