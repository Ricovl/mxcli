import { describe, expect, test } from 'claude-code/testing'

import type { TreeNode } from '../types'
import { assumeDescribable, changesModel, diagramView, nodeId, pageOf, parseDescribe, parseTree, pickMpr, touchesProjectFile, visibleRows } from '../hooks/tree'

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

describe('assumeDescribable (an mxcli older than the describable field)', () => {
  test('a tree with no describable field anywhere offers MDL for named leaves', () => {
    const old: TreeNode[] = [
      {
        label: 'Sales',
        type: 'module',
        qualifiedName: 'Sales',
        children: [
          { label: 'Orders', type: 'folder', children: [{ label: 'ACT_Process', type: 'microflow', qualifiedName: 'Sales.ACT_Process' }] },
        ],
      },
    ]
    const tree = assumeDescribable(old)
    expect(tree[0]!.describable).toBeFalsy() // a module is a container: describing it prints the whole module
    expect(tree[0]!.children![0]!.describable).toBeFalsy() // a folder has no name to describe
    expect(tree[0]!.children![0]!.children![0]!.describable).toBe(true)
  })

  test('a tree from a current mxcli is left as it is', () => {
    expect(assumeDescribable(TREE)).toEqual(TREE)
    const settings = assumeDescribable([{ label: 'Tmpl', type: 'pagetemplate', qualifiedName: 'M.Tmpl', describable: false }, ...TREE])
    expect(settings[0]!.describable).toBe(false)
  })
})

describe('diagramView', () => {
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="40 10 1200 800" width="1200" height="800"><rect/></svg>'

  test('fit shows the whole drawing', () => {
    const v = diagramView(svg, { zoom: 'fit', x: 0 }, 400)
    expect(v.svg).toContain('viewBox="40 10 1200 800"')
    expect(v.canPan).toBe(false)
  })

  test('100% crops to the column at actual size, from the left', () => {
    const v = diagramView(svg, { zoom: 'actual', x: 0 }, 400)
    expect(v.svg).toContain('viewBox="40 10 400 800"')
    expect(v.svg).toContain('width="400"')
    expect(v.canPan).toBe(true)
    expect(v.atStart).toBe(true)
  })

  test('panning moves the window and stops at the right edge', () => {
    expect(diagramView(svg, { zoom: 'actual', x: 1 }, 400).svg).toContain('viewBox="440 10 400 800"')
    const end = diagramView(svg, { zoom: 'actual', x: 9 }, 400)
    expect(end.svg).toContain('viewBox="840 10 400 800"')
    expect(end.atEnd).toBe(true)
  })

  test('a drawing narrower than the column is shown whole at 100%', () => {
    const v = diagramView(svg, { zoom: 'actual', x: 0 }, 2000)
    expect(v.svg).toContain('viewBox="40 10 1200 800"')
    expect(v.canPan).toBe(false)
  })

  test('a drawing with no viewBox is passed through', () => {
    expect(diagramView('<svg><rect/></svg>', { zoom: 'actual', x: 0 }, 400).svg).toBe('<svg><rect/></svg>')
  })
})
