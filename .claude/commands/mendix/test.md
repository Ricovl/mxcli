---
description: Run the app's microflow tests and verify it in the browser
argument-hint: [tests/]
---

# Test App

Verify the running Mendix application using playwright-cli.

## Prerequisites

- App must be running: `mxcli docker run -p app.mpr --wait`
- playwright-cli installed (included in devcontainer)

## First: does the page work? One call, text, no screenshot

```bash
mxcli playwright check /p/Customer_Overview /p/Order_Overview -p app.mpr
mxcli playwright check /p/Customer_Overview -p app.mpr --role User \
    --assert-text 'Customers' --assert-count '.mx-name-dgCustomers [role=row]>=1'
```

One verdict line per page (title, heading, rows, text, console errors, banners);
exit 0 pass, 1 fail, 2 could not check. It signs in itself (`--user/--password`,
`--role`, or a demo user from `-p`) and reuses the saved session. Do not hand-roll
playwright-cli login/goto/sleep/screenshot sequences for this, and screenshot only
for a visual question (`--screenshot out.png`, read once). The playwright-cli
commands below are for **interaction** tests.

## Session lifecycle

Manage the browser session across commands (attach/reuse, check, tear down):

```bash
mxcli playwright open -p app.mpr    # open or attach to the session
mxcli playwright status             # is it live? what page?
mxcli playwright close [--all]      # tear down
```

Typical loop: `open` once and log in, then iterate `mxcli playwright verify … --keep-open`.

## Quick Start

```bash
# Open browser and take snapshot
playwright-cli open http://localhost:8080
playwright-cli snapshot

# Verify a widget exists (page/DOM assertions run in page context -> eval)
playwright-cli eval "() => document.querySelector('.mx-name-widgetName') !== null"

# Take a screenshot
playwright-cli screenshot

# Close when done
playwright-cli close
```

## Login (Security Enabled)

Only needed for playwright-cli interaction sessions (`playwright check` signs in
itself). Dispatch an `input` event after setting `.value`, or Mendix sees an empty field:

```bash
playwright-cli open http://localhost:8080
playwright-cli eval "() => { const el = document.querySelector('#usernameInput'); el.value = 'MxAdmin'; el.dispatchEvent(new Event('input', {bubbles: true})) }"
playwright-cli eval "() => { const el = document.querySelector('#passwordInput'); el.value = 'AdminPassword1!'; el.dispatchEvent(new Event('input', {bubbles: true})) }"
playwright-cli eval "() => document.querySelector('#loginButton').click()"
playwright-cli eval "() => new Promise(r => setTimeout(r, 3000))"
playwright-cli state-save mendix-auth
```

## Full Workflow

```bash
# 1. Apply MDL changes
mxcli exec changes.mdl -p app.mpr

# 2. Build, start, and wait for runtime
mxcli docker run -p app.mpr --fresh --wait

# 3. Check the pages render (exit 1 on any failure)
mxcli playwright check /p/Customer_Overview -p app.mpr

# 3b. Interactions only: drive playwright-cli
playwright-cli open http://localhost:8080
# ... interact and verify ...

# 4. Verify data persistence
mxcli oql -p app.mpr --json "SELECT Name FROM MyModule.Customer"

# 5. Close browser
playwright-cli close
```

## CI/CD Scripts

For regression testing, capture commands in shell scripts:

```bash
# Run a test script
bash tests/verify-customers.sh

# Run all test scripts
for f in tests/verify-*.sh; do bash "$f" || exit 1; done
```

## Tips

- Use `.mx-name-*` selectors from your MDL widget names — they are stable
- Use `eval "() => ..."` for page/DOM assertions (`run-code` runs in Node, where `document` is undefined); `throw new Error(...)` inside the function to fail under `set -e`
- Use `state-save`/`state-load` to persist login across verifications
- In the edit → re-verify loop, pass `mxcli playwright verify … --keep-open` so the next run reuses the warm, logged-in browser instead of cold-launching Chromium (drop any trailing `playwright-cli close` from reused scripts)
- Use `mxcli oql` for data assertions (no npm packages needed)
- The devcontainer ships only the headless shell (no display), so use `playwright-cli screenshot` (and `tracing-start`/`tracing-stop`) for visual debugging — headed mode needs the full Chromium build and a display
- See skill: test-app for full reference
