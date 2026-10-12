// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"io"
	"sync"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// outputGuard wraps an io.Writer with a per-statement line limit.
// It is thread-safe and resettable, allowing reuse across statements.
// When the line limit is exceeded, Write returns an error and all subsequent
// writes are suppressed, preventing runaway output from infinite loops.
type outputGuard struct {
	mu       sync.Mutex
	w        io.Writer
	maxLines int
	lines    int
	exceeded bool
}

func newOutputGuard(w io.Writer, maxLines int) *outputGuard {
	return &outputGuard{w: w, maxLines: maxLines}
}

// reset clears the line count for the next statement.
func (g *outputGuard) reset() {
	g.mu.Lock()
	g.lines = 0
	g.exceeded = false
	g.mu.Unlock()
}

// Write implements io.Writer. Returns an error once the line limit is exceeded.
func (g *outputGuard) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.exceeded {
		return len(p), nil // silently discard; caller already got the error
	}

	g.lines += bytes.Count(p, []byte{'\n'})
	if g.lines > g.maxLines {
		g.exceeded = true
		// Write the current chunk first so output isn't abruptly cut mid-line.
		_, _ = g.w.Write(p)
		return len(p), g.limitError()
	}

	return g.w.Write(p)
}

// exceededError returns the limit error if the current statement went past the
// line limit, or nil. Execute checks it after the statement returns: most
// handlers (every describer) ignore Fprintf's error, so the one Write returned
// never reaches the caller, and without this check the statement succeeded
// with its output silently cut off (mendixlabs/mxcli#1425).
func (g *outputGuard) exceededError() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.exceeded {
		return nil
	}
	return g.limitError()
}

func (g *outputGuard) limitError() error {
	return mdlerrors.NewValidationf("output line limit exceeded (%d lines); output truncated and statement aborted. "+
		"Describe a large module per document (e.g. describe entity / microflow / page ...) instead of all at once", g.maxLines)
}
