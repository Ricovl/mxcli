#!/usr/bin/env bash
# mendixlabs/mxcli#151 — upload through an mxcli-authored file manager, in the
# running app. A model that passes `mx check` proves nothing about whether the
# widget renders, takes a file, or enforces its own settings; this does.
#
#   mxcli new FM --version 11.14.0 --output-dir /root/fm
#   mxcli exec mdl-examples/bug-tests/151-filemanager-upload/app.mdl -p /root/fm/FM.mpr
#   mxcli run --local -p /root/fm/FM.mpr --setup --ensure-db
#   mxcli run --local -p /root/fm/FM.mpr &          # wait for "App is running at"
#   mxcli playwright verify mdl-examples/bug-tests/151-filemanager-upload/ -p /root/fm/FM.mpr
#
# Asserts, for the file manager (Type: Upload, MaxFileSize: 1, AllowedExtensions: '.csv'):
#   1. a small .csv is accepted, saved, listed on the home page, and stored with
#      its exact size (checked through OQL against the running app);
#   2. a 2 MB .csv is refused with the size message and nothing is stored;
#   3. a .txt is refused with the file-type message and nothing is stored.
#
# The runner starts this script in the project directory, with the browser
# session already open. BASE_URL and MXCLI may be overridden.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
MXCLI="${MXCLI:-mxcli}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MPR="$(ls "$PWD"/*.mpr | head -1)"

# Unique names per run, so a rerun against the same database asserts on its
# own uploads only.
RUN="$(date +%s)$$"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
SMALL="$TMP/small-$RUN.csv"
BIG="$TMP/big-$RUN.csv"
TXT="$TMP/notes-$RUN.txt"
cp "$HERE/small.csv" "$SMALL"
cp "$HERE/notes.txt" "$TXT"
head -c 2200000 /dev/zero | tr '\0' 'x' | fold -w 100 > "$BIG"   # over the 1 MB limit

# attempt FILE — open the upload page, choose FILE, press Save. Prints JSON:
# {"stayed": still on the upload page, "shown": name in the widget, "messages": [...]}
attempt() {
  playwright-cli --raw run-code "async page => {
    await page.goto('$BASE_URL/');
    await page.locator('.mx-name-btnNew').click();
    const fm = page.locator('.mx-name-fileManager1');
    await fm.waitFor({ timeout: 30000 });
    await fm.locator('input[type=file]').setInputFiles('$1');
    await page.waitForTimeout(1500);
    const shown = await fm.locator('input.form-control').inputValue();
    await page.locator('.mx-name-btnSave').click();
    await page.waitForTimeout(3000);
    const messages = (await page.locator('.alert, .mx-validation-message, [role=alertdialog]').allInnerTexts())
      .map(s => s.trim()).filter(Boolean);
    return { stayed: (await fm.count()) > 0, shown, messages };
  }"
}

fail() { echo "FAIL: $*" >&2; exit 1; }

# rows NAME — the stored uploads named NAME, as a JSON array ("[]" for none).
# Anything else fails the script: an OQL error must not read as "nothing
# stored", or the two refusal checks below would pass vacuously.
rows() {
  local out
  out="$("$MXCLI" oql -p "$MPR" --json "SELECT Name, Size FROM Docs151.Upload WHERE Name = '$1'" 2>/dev/null)" \
    || fail "OQL query failed for $1"
  case "$out" in
    *'(0 rows)'*) echo '[]' ;;
    '['*) echo "$out" ;;
    *) fail "unexpected OQL output for $1: $out" ;;
  esac
}

count() { local r; r="$(rows "$1")" || exit 1; echo "$r" | { grep -o '"Name"' || true; } | wc -l; }

# 1. Accepted and stored.
out="$(attempt "$SMALL")"; echo "small: $out"
echo "$out" | grep -q '"stayed":false' || fail "small .csv: Save did not close the upload page: $out"
playwright-cli --raw run-code "async page => {
  await page.goto('$BASE_URL/');
  await page.locator('.mx-name-lvUploads').waitFor();
  await page.getByText('Uploaded: $(basename "$SMALL") ($(stat -c %s "$SMALL") bytes)').waitFor({ timeout: 10000 });
}" >/dev/null || fail "small .csv is not listed on the home page with its size"
stored="$(rows "$(basename "$SMALL")")"; echo "stored: $stored"
n="$(count "$(basename "$SMALL")")"
[ "$n" -eq 1 ] || fail "expected one stored row for the small .csv: $stored"
echo "$stored" | grep -q "\"Size\": *\"\\?$(stat -c %s "$SMALL")" || fail "stored size differs from $(stat -c %s "$SMALL") bytes: $stored"

# 2. Over MaxFileSize: refused, nothing stored.
out="$(attempt "$BIG")"; echo "big: $out"
echo "$out" | grep -q '"stayed":true' || fail "2 MB .csv: the page closed, so it was accepted: $out"
echo "$out" | grep -q 'larger than the maximum size (1MB)' || fail "2 MB .csv: no size message: $out"
n="$(count "$(basename "$BIG")")"
[ "$n" -eq 0 ] || fail "2 MB .csv was stored"

# 3. Outside AllowedExtensions: refused, nothing stored.
out="$(attempt "$TXT")"; echo "txt: $out"
echo "$out" | grep -q '"stayed":true' || fail ".txt: the page closed, so it was accepted: $out"
echo "$out" | grep -q 'following types here: .csv' || fail ".txt: no file-type message: $out"
n="$(count "$(basename "$TXT")")"
[ "$n" -eq 0 ] || fail ".txt was stored"

echo "PASS: verify-filemanager-upload"
