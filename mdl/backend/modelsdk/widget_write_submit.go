// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"math"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// submitOnInputDelay is the stored Forms$TextBox/TextArea.SubmitOnInputDelay:
// the authored value, or Studio Pro's default 300 when none was authored. A
// value outside int32 cannot come from a stored widget or pass the builder's
// check; it is clamped rather than wrapped.
func submitOnInputDelay(v *int) int32 {
	if v == nil {
		return pages.DefaultSubmitOnInputDelay
	}
	switch {
	case *v > math.MaxInt32:
		return math.MaxInt32
	case *v < 0:
		return 0
	}
	return int32(*v)
}
