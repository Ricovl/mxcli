---
title: ALTER PAGE Addressing of Pluggable Object-List Items
status: partial
date: 2026-10-17
---

# Proposal: ALTER PAGE Addressing of Pluggable Object-List Items

**Status:** Partial
**Date:** 2026-10-17

Refs ako/mxcli#1326, mendixlabs/mxcli#1453.

## Problem Statement

`ALTER PAGE` can reach a widget *inside* an Accordion group (set, insert
before/after, drop — e94a6df2), but not the group itself:

```
alter page M.P { insert into grp1 { actionbutton btnNew (…) }; };
-- widget "grp1" not found
```

The name never existed. Mendix stores no `Name` on a pluggable widget's
object-list item (`CustomWidgets$WidgetObject`); `create … group grp1 (…)`
parses the name and drops it, and describe prints `group ( … ) { … }`. So an
**empty** group — or a group's header — cannot be filled, and a group cannot be
removed, without `create or replace` of the whole page. The same holds for every
widget with an object list of items holding widgets: Pop-up menu items, and any
container-list style widget the field reported (9 of 12 non-MDL change scripts
on one production app were BSON patches for this class).

The class is the one `docs-wiki/bug-patterns/mutator-addressing.md` describes:
a node the model does not name needs an address that does not depend on a name.

## BSON Structure

Measured on a blank Mendix 11.13.0 app, Accordion
(`com.mendix.widget.web.accordion.Accordion`) authored by mxcli and checked with
`mx check` (0 errors):

```
CustomWidgets$CustomWidget            Name: "acc1"
  Type.ObjectType.PropertyTypes[]
    PropertyKey "groups"   ValueType { Type: "Object", IsList: true,
                                       ObjectType.PropertyTypes[ … ] }
      headerRenderMode  Enumeration
      headerText        TextTemplate      <- the group's visible title
      headerHeading     Enumeration
      headerContent     Widgets           <- second widget slot
      content           Widgets           <- the body
      visible / dynamicClass / initiallyCollapsed   Expression
      loadContent / initialCollapsedState           Enumeration
      collapsed         Attribute
      onToggleCollapsed Action
  Object.Properties[ TypePointer -> "groups" ]
    Value.Objects [marker=2]:
      - CustomWidgets$WidgetObject          (no Name — one per group)
          Properties[ TypePointer -> "content" ].Value.Widgets [marker=2]: [ … ]
          Properties[ TypePointer -> "headerContent" ].Value.Widgets [2]: [ … ]
```

- An item is a `CustomWidgets$WidgetObject` in `Value.Objects`, list marker 2.
- Each widget slot is a `Widgets` property of the item; its `Widgets` key is
  always present (mandatory list, `[2]` when empty), so a write never has to add
  it.
- The item's identity for addressing must therefore be **position** or
  **content**; there is nothing else stored.

## Proposed MDL Syntax

Reuse the navigation-list item address (#1388) for every widget whose children
are unnamed items, and the slot suffix `#1462` introduced (`dataGrid1.controlbar`):

```sql
-- the group's body (bare item = body slot)
alter page M.P { insert into acc1 item@1 { actionbutton btnNew (Caption: 'New', Action: close page) }; };
alter page M.P { insert into acc1 item('One') { … }; };          -- matched on the item's text (headerText)
alter page M.P { insert into acc1 item(btnInGroup) { … }; };      -- the item holding btnInGroup

-- another slot of the item
alter page M.P { insert into acc1 item@1.headercontent { dynamictext t (Content: 'Hi') }; };
alter page M.P { replace acc1 item@1.headercontent with { … }; };
alter page M.P { drop acc1 item@1.headercontent; };              -- empties the slot

-- the item itself
alter page M.P { drop acc1 item@2; };                            -- removes the group
alter page M.P { set (headerText: 'Renamed') on acc1 item@1; };  -- an item property
```

Rules, decided with the maintainer:

1. **One word, `item`, for every list** — not the per-widget describe keyword
   (`group`). The address is about the list's structure, which is the same for
   all of them; a per-widget word would be a second vocabulary to learn and to
   keep in step with describe.
2. **A bare item address means the item's body slot**: the slot CREATE routes
   loose children to — `content` when the item has one, else its first `Widgets`
   property (`defaultItemSlotKey`, `widget_engine.go`). Another slot needs
   `.slot`, spelled as its property key (case-insensitive) or the item-slot
   keyword CREATE accepts for it (`headercontent`).
3. **Legacy table cells are deferred** (`table1 cell(r, c)` sketched below).

One rule follows from the widget's own definition: **a slot the item's
configuration hides is refused, naming the property that enables it.** The
Accordion's editor config hides `headerContent` while `headerRenderMode` is
`'text'` — the default (`hidePropertyIn(…, "groups", …, "headerContent")`,
pinned in `editorconfig_nested_test.go`). Writing widgets into it would store
content Studio Pro hides and does not render, the same hazard #574 settled for
hidden text templates. So `insert into acc1 item@1.headercontent { … }` on a
text-mode group answers "headerContent is hidden while headerRenderMode is
'text' — `set (headerRenderMode: 'custom') on acc1 item@1` first" rather than
writing it.

Shared with the existing forms, so nothing new to learn:
- `@n` is 1-based and, after a selector, chooses among its matches.
- A miss lists the addresses that do resolve; an ambiguous selector is refused
  with the `@n` forms, never resolved to the first match.
- BEFORE/AFTER a **slot** is refused (as for `grid.controlbar`); BEFORE/AFTER an
  **item** inserts an item (`insert after acc1 item@1 { group (…) { … } }`),
  mirroring navigation-list items.

## Implementation Status

Done: steps 1–3, 5–7 (describe slots #1565; shared body rule; grammar; the
resolver for every selector form; INSERT INTO an item's body or slot,
REPLACE / DROP of a slot, DROP of an item, SET of an item property; the
hidden-slot refusal, Open Question 2 answered as "refuse"). Measuring the SET
path found that a mode switch must also keep the texts it hides or shows as
CREATE stores them, and does.

Not done: inserting a NEW item (`insert after acc1 item@1 { group … }`) is
refused; step 4's selector rename was not needed; legacy table cells stay
deferred.

## Implementation Plan

Order matters: describe has to stop misplacing header widgets before an address
that edits them is useful, or describe → edit → exec moves them.

1. **Describe fix (prerequisite, own PR).** `cmd_pages_describe_objectlist.go`
   folds *every* `Widgets` sub-property of an item into `item.Children`, so a
   group's `headerContent` widgets are printed as body and re-created in
   `content` on round-trip. Print the body slot as children and any other
   non-empty slot as a nested `<slotkeyword> { … }` block — the form CREATE
   already parses. Test: round-trip a group with both slots filled.
2. **Shared rule.** Move `defaultItemSlotKey`'s choice into `mdl/types` (as
   `WidgetSlotKeyword` was in #1462) so CREATE, describe and the mutator pick the
   same body slot from one function.
3. **Grammar.** `alterTarget`'s two `ITEM` alternatives gain
   `(DOT identifierOrKeyword)?`. No new keyword.
4. **Selector.** Generalise `backend.NavItemSelector` / `ParseNavItemSelector`
   to carry the optional slot; keep the type name or rename it in a separate
   refactor commit.
5. **Resolver.** Generalise `resolveNavItem` (`navlist.go`) to any object-list
   item: on a CustomWidget, the list is the `Object`-kind, `IsList` property
   (refuse when a widget has more than one such list, naming them, until a
   use case needs a list qualifier). `item('text')` matches the item's
   `TextTemplate` / `String` properties; `item(w)` the item whose slots hold
   widget `w`.
6. **Operations.** `InsertWidget` / `ReplaceWidget` / `DropWidget` /
   `SetWidgetProperty` and `ResolveAlterTarget` route an item address to the
   item and an item.slot address to the slot (reusing `pluggableSlot` from
   `pluggableslot.go`). Inserting an *item* serializes it through the CREATE
   object-list builder, the way navigation-list items are serialized.
7. **Data context.** Widgets inserted into a slot are built in the item's
   enclosing data context — the three child walkers already descend into
   object-list items (#891), so this is a test, not new code.

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/executor/cmd_pages_describe_objectlist.go` | Step 1: print non-body slots as nested blocks |
| `mdl/types/` | Step 2: shared body-slot rule |
| `mdl/executor/widget_engine.go` | Use the shared rule |
| `mdl/grammar/MDLParser.g4`, `mdl/visitor/` | Step 3: `.slot` after an `item` selector |
| `mdl/backend/navitem_selector.go` | Step 4: slot in the selector |
| `mdl/backend/pagemutator/navlist.go`, new `objectlist_items.go` | Steps 5–6 |
| `mdl/backend/pagemutator/alter_target.go`, `mutator.go` | Route the new addresses |
| `docs-site/src/reference/page/alter-page.md`, `.claude/skills/mendix/alter-page/SKILL.md` | Document |

## Version Compatibility

No gate. The storage shape is the pluggable widget framework's
`CustomWidgets$WidgetObject` list, unchanged across the Mendix 10 and 11 versions
mxcli supports, and the address is resolved against the widget's own stored
schema (`ObjectType.PropertyTypes`), not a version table.

## Test Plan

- Mutator unit tests on a fixture with a two-group Accordion: insert into an
  empty body, into `headercontent`, replace/drop a slot, drop an item, set an
  item property, each selector form, `@n` among matches, ambiguity and miss
  messages — and a **control**: each test fails with today's `widget "…" not
  found` before the change.
- Describe round-trip test for step 1 (both slots filled → identical BSON slot
  contents after exec of the describe output).
- `mdl-examples/bug-tests/1453-alter-insert-into-accordion-group.mdl`, executed
  on a blank app and checked with `mxcli docker check` (0 errors) — run on the
  unfixed and fixed builds.
- Pop-up menu: one test that the same address reaches a `basicItem`, proving the
  generalisation rather than an Accordion special case.

## Deferred: Legacy Table Cells

A `Forms$Table` cell is positional (`TopRowIndex`, `LeftColumnIndex`), so its
address would be `table1 cell(row, column)`, 1-based — not `item@n`, which
would depend on storage order. Deferred until (a) a Studio Pro-authored table is
available to confirm the stored shape (a Mendix 11 cell is `Forms$DbTableCell`,
which gen does not model, and `mx convert` refuses `Forms$TableCell`), and (b)
describe prints tables at all. #1463 already lets ALTER reach widgets inside
cells by name.

## Open Questions

1. **Text selector scope.** `item('One')` matches any `TextTemplate`/`String`
   property of the item. For an Accordion that is only `headerText`; a widget
   whose items carry two texts could match on either. Narrow it to the first
   such property in schema order if that turns out confusing.
2. **Hidden-slot refusal vs. auto-enable.** The proposal refuses a write into a
   hidden slot. Switching `headerRenderMode` to `'custom'` automatically would
   be friendlier but changes a property the statement never named; the refusal
   keeps every change explicit.
3. **Widgets with two object lists.** Refused (named) for now; a qualifier such
   as `w groups item@1` can be added if a real widget needs it.
