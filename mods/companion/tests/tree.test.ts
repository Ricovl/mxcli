import { describe, expect, test } from 'claude-code/testing'

import type { TreeNode } from '../types'
import { changesModel, nodeId, pageOf, parseDescribe, parseTree, pickMpr, touchesProjectFile, visibleRows } from '../hooks/tree'

const TREE: TreeNode[] = [
  {
    label: 'Sales',
    type: 'module',
    qualifiedName: 'Sales',
    children: [
      { label: 'Orders', type: 'folder', children: [{ label: 'ACT_Process', type: 'microflow', qualifiedName: 'Sales.ACT_Process', describable: true }] },
      { label: 'Home', type: 'page', qualifiedName: 'Sales.Home', describable: true },
    ],
  },
  { label: 'Settings', type: 'settings', qualifiedName: 'Settings', describable: true },
]

describe('visibleRows', () => {
  test('shows only top-level nodes until one is expanded', () => {
    expect(visibleRows(TREE, new Set()).rows.map(r => r.node.label)).toEqual(['Sales', 'Settings'])
  })

  test('walks into expanded nodes, depth-first, with depth', () => {
    const sales = nodeId('', TREE[0]!)
    const orders = nodeId(sales, TREE[0]!.children![0]!)
    const { rows } = visibleRows(TREE, new Set([sales, orders]))
    expect(rows.map(r => `${r.depth}:${r.node.label}`)).toEqual([
      '0:Sales',
      '1:Orders',
      '2:ACT_Process',
      '1:Home',
      '0:Settings',
    ])
  })

  test('stops at the limit and counts what it left out', () => {
    const sales = nodeId('', TREE[0]!)
    const { rows, more } = visibleRows(TREE, new Set([sales]), 2)
    expect(rows).toHaveLength(2)
    expect(more).toBe(2)
  })
})

describe('parsing mxcli output', () => {
  test('parseTree skips anything printed before the JSON', () => {
    expect(parseTree('Connected to: x\n[{"label":"A","type":"module"}]')).toEqual([{ label: 'A', type: 'module' }])
  })

  test('parseTree refuses output with no list', () => {
    expect(() => parseTree('Error: nope')).toThrow('no JSON')
  })

  test('parseDescribe returns the mdl field', () => {
    expect(parseDescribe('{"name":"M.X","type":"microflow","mdl":"mdl 1;\\n","executable":true}')).toBe('mdl 1;\n')
  })
})

describe('which tool calls change the model', () => {
  test('scripts, writing one-liners, new, rename and marketplace do', () => {
    for (const cmd of [
      './mxcli exec mdlsource/sales.mdl -p app.mpr',
      'mxcli -p app.mpr -c "create entity Sales.Order (Name: string(200))"',
      'cd app && ./mxcli rename entity Sales.A Sales.B -p app.mpr',
      'mxcli new MyApp',
      './mxcli marketplace install 123 -p app.mpr',
    ]) {
      expect(changesModel(cmd)).toBe(true)
    }
  })

  test('reads and unrelated commands do not', () => {
    for (const cmd of [
      './mxcli -p app.mpr -c "show entities"',
      './mxcli describe -p app.mpr microflow Sales.ACT_Process',
      './mxcli check mdlsource/sales.mdl -p app.mpr',
      'git status',
      'echo mxcli-exec',
    ]) {
      expect(changesModel(cmd)).toBe(false)
    }
  })

  test('edits of the project file or its units do', () => {
    expect(touchesProjectFile('/repo/app.mpr')).toBe(true)
    expect(touchesProjectFile('/repo/mprcontents/1a/2b/x.mxunit')).toBe(true)
    expect(touchesProjectFile('/repo/README.md')).toBe(false)
  })
})

describe('pickMpr', () => {
  test('the option wins', () => {
    expect(pickMpr('app/App.mpr', ['Other.mpr'], [])).toEqual({ mpr: 'app/App.mpr' })
  })
  test('one here, then one below, then a choice', () => {
    expect(pickMpr('', ['App.mpr'], ['x/Y.mpr'])).toEqual({ mpr: 'App.mpr' })
    expect(pickMpr('', [], ['x/Y.mpr'])).toEqual({ mpr: 'x/Y.mpr' })
    expect(pickMpr('', [], ['a/A.mpr', 'b/B.mpr'])).toEqual({ candidates: ['a/A.mpr', 'b/B.mpr'] })
    expect(pickMpr('', [], [])).toEqual({ none: true })
  })
})

describe('pageOf', () => {
  test('a page from the top, with what lies below', () => {
    expect(pageOf(100, 0, 10)).toEqual({ first: 0, end: 10, above: 0, below: 90 })
  })
  test('clamps a top past the end to the last full page', () => {
    expect(pageOf(100, 95, 10)).toEqual({ first: 90, end: 100, above: 90, below: 0 })
  })
  test('a short list is one page', () => {
    expect(pageOf(3, 2, 10)).toEqual({ first: 0, end: 3, above: 0, below: 0 })
  })
})
