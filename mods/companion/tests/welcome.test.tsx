import { expect, mock, test } from 'claude-code/testing'
import type { On } from 'claude-code'

const BAND = {
  hasSurvey: false,
  isWorking: false,
  maxRows: 6,
  bodyColumns: 100,
  scroll: { offset: 0, bodyRows: 6 },
  view: {},
} as const

const START = { cwd: '/repo', surface: 'terminal', isInteractive: true } as const

// A project at /repo with one app; `store` is what earlier sessions left.
const world = (on: On, store: Record<string, unknown> = {}) => {
  const opened: string[] = []
  mock.clock(on)
  // The store as its own hooks, so a test can read what the mod left there.
  on('store.get', (_$, e) => ({ value: store[e.key] }))
  on('store.set', (_$, e) => {
    store[e.key] = e.value
    return { value: undefined }
  })
  on('command.register', () => ({ value: { command: 'mxcli-project' } }))
  on('fs.exists', () => ({ value: false }))
  on('fs.list', () => ({ value: [{ name: 'App.mpr', kind: 'file', size: 1, mtimeMs: 0, isLink: false }] }))
  on('ui.open', (_$, e) => {
    opened.push(e.id)
    return { value: { isPlaced: true } }
  })
  on('process.run', () => ({
    value: { exitCode: 0, stdout: '[]', stderr: '', isStdoutTruncated: false, isStderrTruncated: false },
  }))
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  // The engine's own band, drawn when the mod has nothing to say.
  on('ui.render', ($, e) => {
    const { Box } = $.ui.resolve(e)
    return <Box key="engine-band" />
  })
  return { opened, store }
}

for (const surface of ['terminal', 'desktop'] as const) {
  test(`${surface}: the first session in a project offers the Project pane`, async ($, on) => {
    const { opened } = world(on)
    await $.session.start(START)
    const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface, component: 'AbovePrompt', props: BAND })
    expect(await ui.find({ key: 'welcome-open' })).toBeDefined()

    await ui.press({ key: 'welcome-open' })
    expect(opened).toEqual(['mxcli-project'])
    expect(await ui.find({ key: 'welcome-open' })).toBeUndefined()
    await ui.unmount()
  })
}

test('"Not now" hides the band for this project in later sessions too', async ($, on) => {
  const { store } = world(on)
  await $.session.start(START)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'AbovePrompt', props: BAND })
  await ui.press({ key: 'welcome-dismiss' })
  expect(await ui.find({ key: 'welcome-open' })).toBeUndefined()
  expect(store['welcomed:/repo']).toBe(true)
  await ui.unmount()
})

test('a project already welcomed shows no band', async ($, on) => {
  world(on, { 'welcomed:/repo': true })
  await $.session.start(START)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'AbovePrompt', props: BAND })
  expect(await ui.find({ key: 'welcome-open' })).toBeUndefined()
  await ui.unmount()
})

test('opening the pane by command also retires the band', async ($, on) => {
  world(on)
  await $.session.start(START)
  await $.command.run({
    command: 'mxcli-project',
    args: '',
    origin: { kind: 'composer' },
    presentation: { isFullscreen: true, columns: 160 },
  })
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'AbovePrompt', props: BAND })
  expect(await ui.find({ key: 'welcome-open' })).toBeUndefined()
  await ui.unmount()
})
