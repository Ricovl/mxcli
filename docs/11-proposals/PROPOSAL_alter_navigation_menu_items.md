---
title: ALTER NAVIGATION / ALTER MENU — item-level edits to a navigation menu or menu document
status: accepted
date: 2026-10-20
related:
  - navigation-support.md
  - show-describe-menu-documents.md
  - PROPOSAL_offline_sync_configuration.md
  - docs/13-decisions/0010-mdl-canonical-syntax-rules.md
  - docs/13-decisions/0012-mdl-first-and-data-first-editing.md
  - mendixlabs/mxcli#1522
  - ako/mxcli#1404
  - ako/mxcli#1420
---

# Proposal: ALTER NAVIGATION / ALTER MENU — item-level edits to a navigation menu or menu document

**Status:** Accepted (2026-10-20)
**Date:** 2026-10-17 (draft), 2026-10-20 (accepted)

> The maintainer's decisions are recorded in [Decisions](#decisions); the
> sections below are written to them.

## Problem Statement

A navigation profile's menu can only be written whole. `create or modify
navigation <Profile> { … }` replaces the entire menu with the block, so a
statement that means "add one item" deletes every item it does not restate. That
is what the reporter of mendixlabs/mxcli#1522 (ako/mxcli#1404) did: they added
one item and lost the others. PR ako/mxcli#1420 documents the rule ("a `{ }`
menu block replaces the whole menu"), but the only way to change one item is
still the round trip `mxcli syntax navigation.alter` describes: `describe
navigation`, edit the output, then run all of it as `create or modify`.

That round trip is the declarative-only workflow ADR-0012 rejected, and it costs
more here than the length of the menu:

- **The script re-sends, and `check` re-validates, items it never meant to
  touch.** Measured on ako/TestApp (Mendix 11.14.0, Studio Pro-authored). Running
  `describe navigation Responsive` with one caption changed (`'Home'` →
  `'Start'`) is **refused**:

  ```
  ✗ menu item 'Item 4' uses glyph 9993, which the Mendix glyph font does not define … [MDL078]
  Refusing to execute: 1 error(s) above. Nothing was written.
  ```

  The user changed `'Home'`, and the error is on `'Item 4'`. Nobody can edit any
  item in that menu without first repairing one they did not touch, or passing
  `--no-check`.
- **Concurrent edits are lost.** The rewrite writes back whatever the script
  holds. Any item someone added in Studio Pro after the `describe` is deleted.
- **There is no way to remove a page clause.** After ako/mxcli#1420, an omitted
  clause keeps its stored value. That fixes the CE0527 data loss, but MDL now has
  no spelling that clears a login page, a not-found page or a role's home page.
  Leaving the clause out keeps it.

Who benefits: agents and developers editing existing Studio Pro apps (the
data-first mode of ADR-0012), which is where most navigation edits happen.

This proposal adds the data-first half for navigation:
`alter navigation <Profile> { insert / drop / replace / move / set }`. It edits
menu items at any depth, and it sets or clears the profile's page clauses. The
same item operations apply to menu documents (`alter menu M.Menu { … }`),
because their items are the same BSON (see below).

`create or replace` is a deprecated alias of `create or modify` (ADR-0010,
MDL-DEPR001). It keeps that meaning and gains no new one.

## Decisions

Taken by the maintainer on 2026-10-20, resolving the draft's open questions:

1. **Addressing is the path form `'Admin' > 'Users'`** (outer > inner), the form
   mxcli's messages already print item paths in, instead of the draft's
   `'Users' in 'Admin'`. The rest of the scheme stands: a bare caption matches
   anywhere in the tree, a path matches as a **suffix** of an item's path, each
   `>` is a **direct** parent/child step, `@n` belongs to the segment it follows,
   a missing or ambiguous target is refused with the shortest unique address of
   every candidate (in the same `>` form), and there are no positional-only
   addresses. `list navigation menu`'s Address column and every message use the
   same form.
2. **Page clauses**: `set home (page | microflow) M.X [for Role]`, `drop home
   page for Role`, `set login page M.P` / `drop login page`, `set not found page
   M.P` / `drop not found page`. A bare `drop home page` is refused (CE0527).
3. **Menu documents**: yes, `alter menu Module.Menu { … }` with the same item
   operations and no page clauses.
4. **Caption language**: handled **the same way as pages**. A caption is read
   with the rule page `describe` uses and written with the rule `alter page`
   uses (see [Caption language](#caption-language)). Navigation `describe` and
   the address resolver both switch to it; the resulting change to `describe`
   output on multi-language projects is intended.
5. **No `move` for `alter page`** (out of scope). `move` stays inside `alter
   navigation` / `alter menu`.

## BSON Structure

All measured, not assumed. Sources:

- `testdata/expr-checker`, `testdata/pedapp` and an 11.15.0 blank-app fixture
  (Fixture1115), each a Studio Pro-created project;
- ako/TestApp (11.14.0), which has a Studio Pro-authored **nested** menu and every
  action and icon kind;
- each dumped from its `mprcontents/b6/29/b62986ab-….mxunit` (the navigation
  document) and its two `Menus$MenuDocument` units.

`mxcli bson dump` has no `--type` for navigation or menus, so the units were
decoded directly. The field names agree with `modelsdk/gen/menus/types.go` and
`modelsdk/gen/navigation`, and with the reader and writer in
`mdl/backend/modelsdk/navigation_{read,write}.go` and `menu_{read,write}.go`.

### Where the items live

```
Navigation$NavigationDocument                      (one unit per project)
  Profiles: [2, …]
    Navigation$NavigationProfile                   web profile (Responsive, Phone, …Offline)
      Name, Kind, AppTitle (Texts$Text), AppIcon
      HomePage:          Navigation$HomePage { Page, Microflow }
      HomeItems:         [2, Navigation$RoleBasedHomePage { UserRole, Page, Microflow } …]
      LoginPageSettings: Forms$FormSettings { Form, ParameterMappings [2], TitleOverride }
      NotFoundHomepage:  Navigation$NotFoundHomePage { Page, Microflow } | null
      Menu:              Menus$MenuItemCollection { Items: [3, Menus$MenuItem …] }
      OfflineEntityConfigs, ProgressiveWebAppSettings, ThrowPartialSyncError
    Navigation$NativeNavigationProfile             native: BottomBarItems, NOT a Menus$ tree

Menus$MenuDocument                                 (one unit per menu document)
  Name, Documentation, Excluded, ExportLevel
  ItemCollection:    Menus$MenuItemCollection { Items: [3, Menus$MenuItem …] }
```

The item tree is **the same in both documents**. A `Menus$MenuItemCollection`
with `Items` marker 3 holds `Menus$MenuItem` elements. The only difference is the
key that holds the collection: `Menu` on a profile, `ItemCollection` on a menu
document. This is why `alter menu` shares every item operation (Decision 3).

### `Menus$MenuItem`

| Key | Value | Notes |
|---|---|---|
| `$ID` | binary(16) | Unique. **Not referenced from anywhere** (measured below) |
| `$Type` | `Menus$MenuItem` | Storage name equals the qualified name. No mismatch from the CLAUDE.md table applies |
| `Action` | a client action | `Forms$FormAction` (show page), `Forms$MicroflowAction`, `Forms$CallNanoflowClientAction`, `Forms$OpenLinkClientAction`, `Forms$CreateObjectClientAction`, `Forms$SignOutClientAction`, `Forms$NoAction`. These are a button's `Forms$*` names: "Form" is the old word for "Page" |
| `AlternativeText` | `Texts$Text` \| null | null in every observed item. Introduced in 8.12.0 (`gen/menus/version.go`) |
| `Caption` | `Texts$Text { Items: [3, Texts$Translation { LanguageCode, Text } …] }` | **Translatable**: one string per language |
| `Icon` | `Forms$IconCollectionIcon { Image }` \| `Forms$GlyphIcon { Code }` \| `Forms$ImageIcon { Image }` \| null | All three kinds occur in TestApp |
| `Items` | `[3, Menus$MenuItem …]` | Sub-items. Marker 3 even when empty |

Observed key set, identical on every item in all five units: `$ID, $Type,
Action, AlternativeText, Caption, Icon, Items`. That is the property set an
inserted item must carry (CLAUDE.md rule 4, `canon.CompletePropertySets`).
`navMenuItemBson` already writes exactly this set.

### Identity: what an item has and what it lacks

- **No name.** Studio Pro stores none, so an item can only be addressed by
  content (its caption) or by position. Captions are translatable, need not be
  unique, and may be empty (an icon-only item).
- **No `GUID`.** No `GUID`, `PersistentId` or other identity property exists on
  `Menus$MenuItem`, `Menus$MenuItemCollection` or the profile. The
  database-identity rule in CLAUDE.md does not apply, and `canon.identityFields`
  needs no row.
- **No pointers to an item.** The 10 `Menus$MenuItem` `$ID`s of TestApp's
  profile were searched as raw bytes across all of its `mprcontents` units. Each
  occurs exactly once, as its own `$ID`, in the navigation unit. So no other
  element points at a menu item, and an item's `$ID` matters only for diffs in
  version control (ADR-0008). It is not a reference integrity issue.
- **A sub-menu has no action.** Studio Pro stores `Forms$NoAction` on an item
  with sub-items (TestApp `'Item 0'`). `mx check` reports CE0548 ("Items with
  subitems cannot have an action themselves") otherwise. See the comment on
  `keepStoredMenuAction`.

### The page clauses (for `set` / `drop` on the profile)

| Clause | Stored as | "Not set" is |
|---|---|---|
| `home page` / `home microflow` | `HomePage: Navigation$HomePage { Page, Microflow }` | required on a web profile (CE0527 when empty) |
| `home … for <Role>` | one `Navigation$RoleBasedHomePage { UserRole, Page, Microflow }` in `HomeItems` (marker 2) | the element is absent |
| `login page` | `LoginPageSettings: Forms$FormSettings { Form, … }` | **`Form: ""`**, and the element stays (Fixture1115, TestApp) |
| `not found page` | `NotFoundHomepage: Navigation$NotFoundHomePage { Page, Microflow }` | **`null`** (Fixture1115) |

Note the two different spellings of "none": an empty `Form` string, and a null
element. Each clear must write what Studio Pro writes for that field.

### How today's whole-menu rewrite treats identity (measured)

`UpdateNavigationProfile` rebuilds the menu with fresh `$ID`s and writes through
`UpdateRawUnit`, so `canon.Reconcile` (`TransplantIDs`, `CarryTranslations`)
runs. On TestApp, a caption change, and separately an insert at the top plus a
drop in the middle, both kept every untouched item's stored `$ID`, including
the `$ID`s of its caption and action. So identity churn is **not** what this
proposal fixes. What it fixes is the cost, the refusal and the lost concurrent
edits above. The splice below must still be at least as good: untouched items
are passed through byte-identical, not re-paired.

## Proposed MDL Syntax

### Shape

The generic `alter` of ADR-0012 decision 2, `alter <type> <name> { operations }`,
with the operations that type supports. Fragments are written **exactly as
`create or modify navigation` writes items** (`menu item '…' ( OnClick: …,
Icon: … )`, `menu '…' { … }`), so `describe` output can be pasted into a
fragment.

```
alter navigation <Profile> {
  insert (before | after | into) <item> { <menu items> }
  insert { <menu items> }                          -- append to the top level
  replace <item> with { <menu items> }
  move <item> (before | after | into) <item>
  drop <item> [, <item> …]
  set ( Caption: '…', OnClick: <action>, Icon: <icon> | none ) on <item>

  set home (page | microflow) Module.Name [for <UserRole>]
  drop home page for <UserRole>
  set login page Module.Page
  drop login page
  set not found page Module.Page
  drop not found page
}

alter menu Module.Menu {
  -- the item operations above; no page clauses
}
```

A semicolon after an operation is optional, as in `alter page`. The operations
are read up to the next operation keyword, so `drop 'A', 'B'` needs no
terminator either.

### Addressing an item

An item has no name, so it is addressed by **what `describe` prints for it**, its
caption (ADR-0010 R12, ADR-0012 decision 2), as a path written outer to inner:

| Address | Means |
|---|---|
| `'Users'` | the item captioned `Users`, anywhere in the tree |
| `'Admin' > 'Users'` | the item `Users` whose **parent** is captioned `Admin` |
| `'Settings' > 'Admin' > 'Users'` | the same, one level further out |
| `'Users'@2` | the 2nd item captioned `Users`, in describe order |
| `'Admin'@2 > 'Users'` | `Users` inside the 2nd item captioned `Admin` |
| `'Admin' > 'Users'@2` | the 2nd `Users` among the sub-items of the items captioned `Admin` |
| `''` | an item with an empty caption (icon-only), usually with `@n` |

Rules:

1. **Captions match exactly.** The comparison is case-sensitive, after MDL
   unquoting (`'it''s'`), against the caption string `describe` prints. The
   resolver reads a caption with the **same function** `describe` does (see
   [Caption language](#caption-language)), so "copy what describe printed" is
   true by construction.
2. **The search covers the whole tree, in describe order.** That order is
   pre-order: an item, then its sub-items, then its next sibling. This matches
   how `alter workflow` and `alter microflow` resolve a caption anywhere in the
   document.
3. **A path matches as a suffix, one direct step per `>`.** The first segment
   matches anywhere in the tree; each following segment matches only the
   **direct** sub-items of what the segment before it matched. So
   `'Admin' > 'Users'` does not reach a `Users` two levels below `Admin`, and an
   address never changes meaning when an unrelated level is added elsewhere. The
   chain need not start at the top level.
4. **`@n` belongs to the segment it follows.** It picks the n-th match of that
   segment, in describe order, among the matches the segments before it allow.
   This keeps every address compositional, with no rule about which segment
   `@n` applies to. A segment without `@n` keeps all its matches, so an
   intermediate segment may be ambiguous as long as the whole path is not
   (`'Admin' > 'Users'` resolves when only one of two `Admin` menus has a
   `Users`).
5. **mxcli never guesses** (ADR-0012). A target that matches nothing is an
   error, and so is one that matches several items. Each ambiguity lists every
   candidate with the **shortest unique address** that reaches it, in the same
   `>` form:

   ```
   alter navigation Responsive: menu item 'Users' is ambiguous: it matches 2 items — write one of these:
     'Admin' > 'Users'
     'Reports' > 'Users'
   ```

   The shortest unique address is the shortest suffix of the item's caption path
   that resolves to it alone; when no suffix does (siblings sharing a caption),
   it is its parent's shortest address followed by `> 'Caption'@n`, n counting
   only the parent's sub-items with that caption (`'Caption'@n` over the whole
   tree for a top-level item). Every address printed is checked to resolve back
   to its item.

   A miss lists the captions at the level searched and any item whose caption
   differs only in letter case.
6. **Positional-only addresses are not offered** (`item@3`, index paths `1.2`).
   ADR-0012 rejected them as brittle under any unrelated edit. `@n` only chooses
   among items that share a caption.

`describe navigation` prints no addresses: R12 forbids printing positions or
labels Mendix does not store, and the captions it already prints are the
addresses. What a user needs when captions repeat is a listing that shows them.
`list navigation menu [<Profile>]` becomes a table with an **Address** column
printing the shortest unique address of every item. The ambiguity error prints
the same strings, and so do the existing messages that name a menu item by its
path (`menu item 'Admin' > 'Users'`).

### Caption language

A caption is a `Texts$Text`: one `Texts$Translation { LanguageCode, Text }` per
language. Before this proposal navigation read it differently from every other
text:

| | Rule | Where |
|---|---|---|
| Page / widget describe | the **project's default language**; a default-language entry that is present but empty is printed as `''`; else `en_US`; else the first non-empty entry | `selectTranslationText(items, describeDefaultLanguage(ctx))`, `mdl/executor/describe_language.go` (#702, ako/mxcli#705) |
| `alter page … set Caption` / Title / Label | update the **authoring language's** entry (= the default language), or add one; every other language's entry is kept | `pagemutator.setTextsTextTranslation` (CE4899 case) |
| `create page` caption | one translation, in the authoring language | `model.AuthoringLanguage()` |
| Navigation / menu describe (before) | the **first non-empty** translation, in document order | `textOf`, `navigation_read.go` |

So on a project whose default is `nl_NL` with an item stored `en_US 'Home'`,
`nl_NL 'Start'`, page-style reading prints `'Start'` and navigation printed
`'Home'`.

Decision 4 makes navigation follow pages, with **one** reading function shared by
the three consumers:

- **Read** (describe, `list navigation menu`, the address resolver, `check`):
  the page `describe` rule above. It moves from the executor to the `model`
  package (`model.SelectTranslation`) so the backend's navigation reader, the
  menu splice and the page describer call the same code; `selectTranslationText`
  delegates to it unchanged.
- **Write** `set ( Caption: '…' ) on <item>`: the `alter page` rule, through the
  same helper (it moves from `pagemutator` to `bsonnav`): the authoring
  language's translation is updated or added, every other language is kept.
- **Write** an inserted or replacing item: one translation in the authoring
  language, as `create navigation` already writes.

The executor resolves the project's default language before reading or writing
a navigation document, as `alter page` does (`authoringLanguage(ctx)`), since
the backend reads it through `model.AuthoringLanguage()`.

**Effect on describe (intended).** On a single-language project nothing
changes. On a multi-language project, navigation and menu `describe` now print
the default-language caption instead of the first stored one, and an item whose
default-language caption is empty prints `''` — exactly what `describe page`
prints for a button with the same texts. The reader also stops dropping an item
whose caption is empty and whose action is not a page: such an item (an
icon-only microflow item, say) was invisible to `describe` and so deleted by a
`describe` → `create or modify` round trip; it is now printed as
`menu item '' ( … )` and addressable as `''`.

Out of scope: two page **address** forms still compare other texts — a grid
column's `column('Caption')` matches the first non-empty translation, and a
navigation-list widget's `item('Text')` matches any language. They are page
targeting rules, not navigation, and are left as they are.

### Item operations

- **`insert before | after <item> { … }`** places the fragment's items as
  siblings of the target.
- **`insert into <item> { … }`** appends them as the target's last sub-items. To
  place one first, use `insert before` on the first sub-item.
- **`insert { … }`** with no position appends to the top level. It works like
  `set ( … )` without `on`, which addresses the document itself in `alter page`.
  It is also the only way to add to an empty menu.
- **`replace <item> with { … }`** puts the fragment's items (one or more) where
  the target was. The target's sub-items go with it, unless the fragment states
  them again.
- **`move <item> before | after | into <item>`** relocates the stored item
  **with its bytes**: its `$ID`, its sub-items, every caption translation, and an
  action MDL cannot spell (a page title override, a non-Web link type). Moving is
  not `drop` + `insert`, which would lose all of that. A move into the item's own
  subtree, or relative to itself, is refused.
- **`drop <item>, …`** removes each item and its sub-items.
- **`set ( Key: value, … ) on <item>`** changes properties in place, keeping the
  item's `$ID` and everything not named. The keys are the item property names
  `create` already uses, plus `Caption`, which `create` writes positionally:
  - `Caption: '…'` writes the authoring language's translation; other languages
    stay as they are, as when the caption is edited in Studio Pro.
  - `OnClick: <action>` takes the same action expression `create` takes. `OnClick:
    nothing` stores `Forms$NoAction`.
  - `Icon: Module.Collection.name | glyph <n> | image Module.Images.name | none`.
    `none` follows the `Authentication: none` precedent and stores a null icon.

### Profile operations (`alter navigation` only)

The profile's settings are **clauses** in `create`, not a property list (ako/mxcli#754, recorded under R2 of
[PROPOSAL_mdl_beta_syntax_freeze.md](PROPOSAL_mdl_beta_syntax_freeze.md): "the profile's
clauses … stay clauses for now"). So each
operation is that clause written exactly as in `create`, preceded by `set` or
`drop`. A clause line `describe` printed becomes an operation by prefixing
`set`.

| Operation | Writes |
|---|---|
| `set home page M.P` / `set home microflow M.F` | `HomePage`'s `Page` / `Microflow`, in the stored element (its `$ID` stays) |
| `set home page M.P for Role` | that role's `RoleBasedHomePage`: the stored one is updated in place, or one is appended; other roles are untouched |
| `drop home page for Role` | removes that role's element from `HomeItems`. The kind word does not have to match: this removes a microflow home too. A role with no home page is refused |
| `set login page M.P` / `drop login page` | `LoginPageSettings.Form` = `M.P` / `""` (the element and its `$ID` stay) |
| `set not found page M.P` / `drop not found page` | `NotFoundHomepage` = element / `null` |

There is **no `drop home page`** without `for`. The default home page is required
on a web profile (CE0527), so the statement refuses it. Use `set home …` to
change it.

### One complete example

Given what `describe navigation Responsive` prints:

```mdl
create or modify navigation Responsive
  home page Shop.Home_Web
  home page Shop.AdminHome for Administrator
  login page Administration.Login
  not found page Shop.NotFound
{
  menu item 'Home' ( OnClick: show page Shop.Home_Web, Icon: Atlas_Core.Atlas.home )
  menu 'Admin' ( Icon: Atlas_Core.Atlas.cog ) {
    menu item 'Users' ( OnClick: show page Administration.Account_Overview )
    menu item 'Old import' ( OnClick: call microflow Shop.ACT_LegacyImport )
  }
  menu 'Reports' {
    menu item 'Users' ( OnClick: show page Shop.Report_Users )
  }
  menu item 'Help' ( OnClick: open link 'https://example.com/help' )
  menu item 'Log out' ( OnClick: sign out )
};
```

one statement makes several unrelated edits and touches nothing else:

```mdl
alter navigation Responsive {
  insert after 'Home' {
    menu item 'Orders' ( OnClick: show page Shop.Order_Overview, Icon: Atlas_Core.Atlas."list-bullets" )
  }
  insert into 'Admin' {
    menu item 'Audit log' ( OnClick: show page Shop.AuditLog_Overview )
  }
  set ( Caption: 'User accounts' ) on 'Admin' > 'Users';
  set ( OnClick: show page Shop.Report_UserActivity ) on 'Reports' > 'Users';
  replace 'Help' with {
    menu 'Help' {
      menu item 'Documentation' ( OnClick: open link 'https://example.com/docs' )
      menu item 'Contact' ( OnClick: show page Shop.Contact )
    }
  }
  move 'Log out' after 'Help';
  drop 'Old import';
  set login page Shop.CustomLogin;
  drop not found page;
}
```

A bare `'Users'` in that statement is refused, because it matches two items. The
error offers `'Admin' > 'Users'` and `'Reports' > 'Users'`.

## Semantics

- **Atomic per statement.** The operations apply in order to an in-memory copy
  of the stored profile (or menu document). Any refusal discards the copy, and
  nothing is written. The statement makes one write.
- **Sequential resolution.** Each target resolves against the menu as the
  earlier operations of the same statement left it. This matches what `alter
  page` does (`resolveAlterPageTargets` runs per operation), so an item inserted
  by one operation can be the target of the next.
- **Splice, not rebuild (ADR-0012 decision 3).** Untouched items, and every
  untouched key of a `set` target, are carried as the stored bytes. Only new
  nodes are placed. New nodes are fragment items and their caption, translation,
  action and icon elements, the translation a `set Caption` adds, and the
  role-home, login or not-found element a `set` creates where none is stored.
  They get fresh `$ID`s (CLAUDE.md rule 3), and an `Items` marker of 3. No
  existing `$ID` is ever rewritten (CLAUDE.md rule 1).
- **The write.** It goes through `canon.Reconcile` via the patch entry point
  `UpdateRawUnitPatch` (CLAUDE.md rule 2), as `alter microflow` does. Element
  `$ID` re-pairing and translation carry are off, because the splice already owns
  both. Re-pairing after a `drop` could move a surviving item's `$ID` onto a
  different item, and carrying translations would restore a caption on purpose
  removed. Elision and the storage-GUID guard apply as for every write, so an
  `alter` that changes nothing (a `set` to the stored value, say) writes nothing
  and reports so.
- **`replace` is new content.** The replacement items get fresh `$ID`s. For each
  replacement item whose caption text equals a replaced item's (paired by
  caption at each level, as `create or modify` pairs items), the splice carries
  that caption's other-language translations. The global carry is off for a
  patch, so without this a translated app would lose its translations through a
  `replace` that did not change the caption.
- **Refusals.** Each one names the statement, the operation and the target:
  - a target that is missing or ambiguous, or an `@n` out of range (the listing
    above);
  - `insert into` / `move into` an item whose action is not `Forms$NoAction`, and
    `set ( OnClick: … )` other than `nothing` on an item with sub-items. These
    are CE0548. The refusal says how to make the change: `set ( OnClick: nothing
    ) on …` first;
  - `move` into the item's own subtree, or relative to itself;
  - `drop home page` without `for`, which would cause CE0527, and `drop home
    page for` a role that has none;
  - a **native** profile. Its menu is a bottom bar of a different `$Type`
    (`BottomBarItems`) that mxcli does not write, and its home pages are
    different elements; `alter navigation` refuses the profile and points at
    `create or modify navigation <Profile> home …`, which writes a native
    profile's home pages and keeps everything else;
  - a profile the project does not have. `alter` does not create one. The
    message points at `create or modify navigation`, which does;
  - an action a menu item cannot carry (save, delete, close page, complete
    task). This is the same refusal `create` makes, from the same visitor rule
    and `menuActionBuilder`.
- **Only what the statement says is validated.** The fragment items and `set`
  values go through the checks `create` applies: references (CE1571), icons
  (MDL077/MDL078) and roles (the user-role check of mendixlabs/mxcli#1001).
  Untouched stored items are not re-validated. That is the point of the
  TestApp measurement above: a stored glyph 9993 must not block an edit to
  `'Home'`.
- **`check` and `exec` agree.** `mxcli check` without a project checks syntax and
  the fragments. `check -p --references` runs the **same function `exec`
  runs** over the stored document, opened for mutation and never saved, the
  way `validate_alter_page_refs.go` does for pages: every target `exec` would
  refuse, `check` reports with the same message. Successive `alter`s of one
  document in a script are applied to the same unsaved copy, so a later
  statement sees what an earlier one inserted. A document the script creates or
  rewrites with `create or modify` is "could not establish" and stays silent.
- **A duplicate caption is allowed.** Studio Pro allows two siblings with one
  caption, so an insert that creates a duplicate is written; the item is then
  addressed with `@n`, which `list navigation menu` prints.

## Implementation Plan

Order. Each step is a separate commit (CLAUDE.md, scope and atomicity):

1. **Refactor:** rename the AST/executor's `AlterNavigationStmt` /
   `execAlterNavigation`, which today implement **`create or modify`**, to
   `CreateNavigationStmt` / `execCreateNavigation`. This frees the name. No
   behaviour change.
2. **Refactor:** one caption rule. `model.SelectTranslation` (from
   `selectTranslationText`) and `bsonnav.SetTextsTextTranslation` (from
   `pagemutator.setTextsTextTranslation`). No behaviour change.
3. **Caption language for navigation** (Decision 4): `textOf` reads with
   `model.SelectTranslation`; describe/list prime the language; the reader keeps
   empty-caption items.
4. **Address and resolver** (`mdl/types`, `mdl/backend`), with unit tests.
5. **Menu splice** (`mdl/backend/menumutator`) and the backend mutators, with
   unit tests on encoded documents.
6. **Grammar, AST, visitor** for `alter navigation` / `alter menu`.
7. **Executor**: `exec`, `check -p`, `list navigation menu` Address column.
8. Docs, skill and syntax topics; doctype tests and the #1522 bug test.

### Files to modify/create

| File | Change |
|---|---|
| `mdl/grammar/MDLParser.g4` | `alterStatement`: `ALTER NAVIGATION (qualifiedName \| IDENTIFIER) LBRACE alterNavigationOperation+ RBRACE` and `ALTER MENU_KW qualifiedName LBRACE alterMenuOperation+ RBRACE`. New `alterMenuOperation` (insert/replace/move/drop/set over `navMenuChildren` fragments) and `alterNavigationOperation` (`alterMenuOperation` \| the page clauses after `SET`/`DROP`). New `alterMenuTarget : alterMenuTargetSegment (GREATER_THAN alterMenuTargetSegment)*`, a segment being `STRING_LITERAL (AT NUMBER_LITERAL)?`. `MOVE`, `INSERT`, `REPLACE`, `ITEM`, `CAPTION` and `>` are existing tokens, so the lexer is unchanged |
| `mdl/ast/ast_navigation.go` | `AlterNavigationStmt { ProfileName; Operations }`, `AlterMenuStmt { Name; Operations }` and op nodes `InsertMenuItemsOp`, `ReplaceMenuItemOp`, `MoveMenuItemOp`, `DropMenuItemsOp`, `SetMenuItemOp`, `SetNavPageClauseOp`, `DropNavPageClauseOp` |
| `mdl/types/menu_address.go` (new) | `MenuItemAddress { Segments []{Caption; Ordinal} }` with `String()` in the `>` form |
| `mdl/visitor/visitor_alter_navigation.go` (new) | Builds the AST. `set ( … )` key table (`Caption`, `OnClick`, `Icon`) with repeated/unknown-key errors. Fragments reuse `buildNavMenuItemDef` |
| `mdl/backend/menu_address.go` (new) | `MenuTreeNode`, `ResolveMenuItemAddress(addr, tree)` and `ShortestMenuItemAddress`, returning a `MenuAddressError` listing the shortest unique addresses |
| `mdl/backend/navigation.go` | `MenuMutator` (`MenuTree`, `InsertItems`, `ReplaceItem`, `MoveItem`, `DropItem`, `SetItemCaption`, `SetItemAction`, `SetItemIcon`, `Save`) and `NavigationProfileMutator` (embeds it, adds `SetHomePage`, `DropRoleHomePage`, `SetLoginPage`, `SetNotFoundPage`). `OpenNavigationProfileForMutation(profile)` and `OpenMenuDocumentForMutation(id)` on `NavigationBackend`. The mutators take resolved positions, not addresses: resolution and refusals live in one executor function both `exec` and `check` call |
| `mdl/backend/menumutator/` (new) | The bson.D splice over a `Menus$MenuItemCollection`, parameterised by where the collection sits (`Profiles[i].Menu` or `ItemCollection`), so both documents share it; the page-clause edits; local caption-translation carry for `replace` |
| `mdl/backend/modelsdk/navigation_mutator.go` (new) | Opens the raw unit, builds new items with the existing `navMenuItemBson`, and saves via `writer.UpdateRawUnitPatch` |
| `mdl/backend/mock/`, `mdl/backend/mcp/` | Mock mutator hooks for executor tests. The MCP backend reports the operation as unsupported (regenerate `unsupported_gen.go` / `unimplemented_gen.go`) |
| `mdl/executor/cmd_alter_navigation.go` (new) | Resolve, refuse, apply and save in one pass. Fragments converted with `convertMenuItemDefs` + `menuActionBuilder`. Role resolution reuses the #1001 check. Write report via `ctx.reportWrite` |
| `mdl/executor/validate_alter_navigation_refs.go` (new) | `check -p --references`: the exec function over unsaved mutators |
| `mdl/executor/cmd_navigation.go` | `list navigation menu`: a table with the Address column |
| `cmd/mxcli/syntax/features_misc.go` | Rewrite `navigation.alter` (today it documents the round trip). Fix `navigation.menu-document`. Add `menu.alter` |
| `docs-site/src/reference/navigation/alter-navigation.md` | The page titled ALTER NAVIGATION documents `CREATE OR REPLACE NAVIGATION`. Move that to `create-navigation.md` and document `alter` here |
| `.claude/skills/mendix/manage-navigation/SKILL.md` | "Change one item" section: `alter`, not describe/edit/replace. Then `make sync-skills` |
| `docs/01-project/MDL_QUICK_REFERENCE.md` | Rows for `alter navigation` / `alter menu` |
| `mdl-examples/doctype-tests/` | An `alter navigation` / `alter menu` script, one block per operation (Test Plan) |

## Version Compatibility

`Menus$MenuItem`, `Menus$MenuItemCollection` and every key used above exist in
every Mendix version mxcli supports. `AlternativeText` is the only versioned item
property (8.12.0), and this proposal neither reads nor writes it. The
not-found page (`Navigation$NotFoundHomePage`) and `ThrowPartialSyncError`
already exist in the `create` path, which has its own gating. **No new version
gate.** The tests run on 11.6.6 and 11.15.0 because those are the two mxbuild
versions the `create` path was measured on (ako/mxcli#1420), not because either
introduced anything.

## Test Plan

- **Grammar and visitor** (`mdl/visitor/`): every address form (`'X'`,
  `'Y' > 'X'`, chains, `@n` on each segment, `''`), every operation, and the
  `set ( … )` key errors (unknown, repeated, wrong shape).
- **Resolver** (`mdl/backend/`): unique, missing and ambiguous targets, suffix
  and direct-step matching, `@n` per segment and out of range. The ambiguity
  listing is the shortest unique addresses, and each listed address resolves to
  exactly the item it names (checked over every item of generated trees).
- **Caption language**: a multi-language item reads the default language, an
  empty default-language entry reads `''`, `en_US` is the fallback; describe of
  such a menu changes accordingly (a test that fails against `textOf`'s
  first-non-empty rule).
- **Mutator, on encoded documents** (`mdl/backend/menumutator/`):
  - each operation produces the expected tree. Untouched items are
    **byte-identical** to the input, including their caption translations;
  - `move` keeps the moved item's bytes (`$ID`, translations, a kept action);
  - `set Caption` changes one translation and keeps the others;
  - `replace` carries the other-language translations of a kept caption.
- **Identity carry, with a control** (CLAUDE.md: "any test asserting nothing
  changed must include a control"). After `drop`, every surviving item keeps its
  stored `$ID` and bytes. The **control** rebuilds the same post-drop tree with
  `navMenuItemBson`, which must fail the same assertion, proving the assertion
  can detect a rebuild.
- **Elision.** An `alter` whose `set` restates stored values writes nothing.
- **Executor** (`mdl/executor/`, `MockBackend`): the refusals in Semantics leave
  the document unwritten. The statement is atomic (a refusal in a later
  operation means the earlier ones are not written). `check -p --references` and
  `exec` produce the same message for each refusal.
- **The reported case.** Adding one item to a menu with `alter navigation …
  { insert … }` keeps every other item, without restating the menu
  (`mdl-examples/bug-tests/1522-…`).
- **`mx check` on 11.6.6 and 11.15.0** for each operation on fixture copies:
  insert (before/after/into/top level), replace, move, drop, set, nested items,
  each profile `set`/`drop`, and `alter menu` on a menu document. Expected: 0
  errors; and a `describe` → `create or modify` after the alter is "unchanged".

## Open Questions

1. **`AlternativeText`.** It exists (8.12+) but is null in every observed item,
   and `create` has no key for it. Leave it out (recommended) until a Studio
   Pro-authored example with a value is available.
2. **A listing for menu-document items.** `list navigation menu` gains the
   Address column; there is no `list` statement for a menu document's items, so
   a menu document's addresses are only shown by the ambiguity error. A `list
   menu items Module.Menu` (or an Address column on some existing listing) is a
   small follow-up if wanted.
