import { expect, mock, test } from 'claude-code/testing'
import type { On } from 'claude-code'

const TREE = JSON.stringify([
  {
    label: 'Sales',
    type: 'module',
    qualifiedName: 'Sales',
    children: [
      { label: 'ACT_Process', type: 'microflow', qualifiedName: 'Sales.ACT_Process', describable: true },
      { label: 'Tmpl', type: 'pagetemplate', qualifiedName: 'Sales.Tmpl' },
    ],
  },
])

const MDL = 'mdl 1;\ncreate microflow Sales.ACT_Process ()\nbegin\nend;\n'

const PANE_PROPS = {
  title: 'Project',
  isFocused: true,
  bodyColumns: 80,
  placement: 'dock',
  scroll: { offset: 0, bodyRows: 40 },
  view: {},
} as const

// The world beneath the mod: one app here, and an mxcli that answers
// project-tree and describe. `calls` records every argv it was run with.
const world = (on: On, opts: { describeFails?: boolean } = {}) => {
  const calls: string[][] = []
  const clock = mock.clock(on)
  on('fs.exists', () => ({ value: false }))
  on('fs.list', () => ({ value: [{ name: 'App.mpr', kind: 'file', size: 1, mtimeMs: 0, isLink: false }] }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('command.register', () => ({ value: { command: 'mxcli-project' } }))
  on('process.run', (_$, e) => ({ value: answer([...e.argv]) }))
  const answer = (argv: string[]) => {
    calls.push(argv)
    if (argv[1] === 'project-tree') {
      return { exitCode: 0, stdout: TREE, stderr: '', isStdoutTruncated: false, isStderrTruncated: false }
    }
    if (argv[1] === 'describe') {
      return opts.describeFails
        ? { exitCode: 1, stdout: '', stderr: 'Error: not found\n', isStdoutTruncated: false, isStderrTruncated: false }
        : {
            exitCode: 0,
            stdout: JSON.stringify({ name: 'Sales.ACT_Process', type: 'microflow', mdl: MDL, executable: true }),
            stderr: '',
            isStdoutTruncated: false,
            isStderrTruncated: false,
          }
    }
    return { exitCode: 2, stdout: '', stderr: 'unexpected', isStdoutTruncated: false, isStderrTruncated: false }
  }
  return { calls, clock }
}

// The person typing /mxcli-project at the prompt.
const RUN = {
  command: 'mxcli-project',
  args: '',
  origin: { kind: 'composer' },
  presentation: { isFullscreen: true, columns: 160 },
} as const

const rowKey = (...path: string[]) => path.map(p => `/${p}`).join('')

for (const surface of ['terminal', 'desktop'] as const) {
  test(`${surface}: opens, expands a module and shows a document's MDL`, async ($, on) => {
    const { calls } = world(on)
    await $.command.run(RUN)
    const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface, component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })

    expect(calls[0]).toEqual(['mxcli', 'project-tree', '-p', 'App.mpr'])
    expect(await ui.find({ text: /ACT_Process/ })).toBeUndefined()

    await ui.press({ key: rowKey('module:Sales') })
    expect(await ui.find({ text: /ACT_Process/ })).toBeDefined()

    await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })
    expect(calls.at(-1)).toEqual(['mxcli', 'describe', '-p', 'App.mpr', 'microflow', 'Sales.ACT_Process', '--json'])
    const code = await ui.find({ type: 'Code' })
    expect(code).toBeDefined()
    expect(JSON.stringify(code)).toContain('create microflow Sales.ACT_Process')
    await ui.unmount()
  })

  test(`${surface}: a type without MDL is not described`, async ($, on) => {
    const { calls } = world(on)
    await $.command.run(RUN)
    const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface, component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })
    await ui.press({ key: rowKey('module:Sales') })
    await ui.press({ key: rowKey('module:Sales', 'pagetemplate:Tmpl') })
    expect(calls.some(c => c[1] === 'describe')).toBe(false)
    expect(await ui.find({ text: /has no MDL source/ })).toBeDefined()
    await ui.unmount()
  })

  test(`${surface}: a describe failure is shown, not a blank pane`, async ($, on) => {
    world(on, { describeFails: true })
    await $.command.run(RUN)
    const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface, component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })
    await ui.press({ key: rowKey('module:Sales') })
    await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })
    expect(await ui.find({ text: /not found/ })).toBeDefined()
    await ui.unmount()
  })
}

test('an agent write reloads the tree once; a read does not', async ($, on) => {
  const { calls, clock } = world(on)
  on('tool.call', () => ({ result: { stdout: '', stderr: '', interrupted: false }, text: 'ok' }))
  await $.command.run(RUN)
  await clock.settle()
  const trees = () => calls.filter(c => c[1] === 'project-tree').length
  expect(trees()).toBe(1)

  await $.tool.call({ tool: 'Bash', command: './mxcli -p App.mpr -c "show entities"' })
  await clock.advance(5_000)
  expect(trees()).toBe(1)

  await $.tool.call({ tool: 'Bash', command: './mxcli exec a.mdl -p App.mpr' })
  await $.tool.call({ tool: 'Bash', command: './mxcli exec b.mdl -p App.mpr' })
  await clock.advance(5_000)
  expect(trees()).toBe(2)
})

test('going back to a document reuses its MDL until the model changes', async ($, on) => {
  const { calls, clock } = world(on)
  on('tool.call', () => ({ result: { stdout: '', stderr: '', interrupted: false }, text: 'ok' }))
  await $.command.run(RUN)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })
  const describes = () => calls.filter(c => c[1] === 'describe').length
  await ui.press({ key: rowKey('module:Sales') })
  await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })
  await ui.press({ key: rowKey('module:Sales', 'pagetemplate:Tmpl') })
  await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })
  expect(describes()).toBe(1)
  expect(await ui.find({ type: 'Code' })).toBeDefined()

  // The agent changes the model: the reload re-describes the selection afresh.
  await $.tool.call({ tool: 'Bash', command: './mxcli exec a.mdl -p App.mpr' })
  await clock.advance(5_000)
  expect(describes()).toBe(2)
  await ui.unmount()
})
