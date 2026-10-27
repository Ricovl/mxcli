// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// settingsBsonToMDL renders the project-settings unit as DESCRIBE SETTINGS
// does, with secret values masked. There was no case for it, so it fell into
// the generic one-line header, which reads the same before and after any
// ALTER SETTINGS: "1 modified" and no hunk (mendixlabs/mxcli#1602).
func settingsBsonToMDL(ctx *ExecContext, raw map[string]any, unitType, qualifiedName string) string {
	ps := ctx.Backend.ParseProjectSettingsFromRaw(raw)
	if ps == nil {
		return fmt.Sprintf("-- %s: %s\n-- Type: %s\n-- (settings did not decode)", simplifyTypeName(unitType), qualifiedName, unitType)
	}
	out := renderToString(ctx, func() { writeProjectSettingsMDL(ctx, ps, maskedSettingValue) })
	return strings.TrimRight(out, "\n")
}

var (
	settingMaskKeyOnce sync.Once
	settingMaskKey     []byte
)

// maskedSettingValue stands in for a configuration constant's value in a diff:
// those often hold secrets (mendixlabs/mxcli#1602), and diff output is what gets
// pasted into reviews. A fixed mask would make a changed value render the same
// on both sides — the silent diff this exists to fix — so the mask carries a
// fingerprint: an HMAC under a key drawn once per process and never printed.
// Equal values match within one diff, different ones differ, and nothing in
// the output can be checked against a guess afterwards.
func maskedSettingValue(v string) string {
	if v == "" {
		return "''"
	}
	settingMaskKeyOnce.Do(func() {
		settingMaskKey = make([]byte, 32)
		if _, err := rand.Read(settingMaskKey); err != nil {
			panic("diff-local: no randomness for the value mask: " + err.Error())
		}
	})
	mac := hmac.New(sha256.New, settingMaskKey)
	mac.Write([]byte(v))
	return "<masked #" + hex.EncodeToString(mac.Sum(nil))[:8] + ">"
}
