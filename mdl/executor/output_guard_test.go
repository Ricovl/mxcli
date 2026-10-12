// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A statement whose output passes the per-statement line limit must fail, not
// exit 0 with the text cut off mid-statement. Describers ignore Fprintf's
// error, so the guard's own error never reached the caller and
// `describe module … with all` stopped at exactly 10,000 lines with no
// warning (mendixlabs/mxcli#1425). The statement under the limit is the
// control.
func TestExecuteReportsOutputLineLimitExceeded(t *testing.T) {
	for _, tc := range []struct {
		name    string
		lines   int
		wantErr bool
	}{
		{"under the limit", maxOutputLines, false},
		{"over the limit", maxOutputLines + 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			e := New(&out)
			e.registry.handlers[reflect.TypeOf(&ast.DescribeStmt{})] = func(ctx *ExecContext, _ ast.Statement) error {
				for i := 0; i < tc.lines; i++ {
					fmt.Fprintf(ctx.Output, "line %d\n", i) // error ignored, as describers do
				}
				return nil
			}
			err := e.Execute(&ast.DescribeStmt{})
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("Execute: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Execute returned nil after %d lines; output silently truncated at %d", tc.lines, strings.Count(out.String(), "\n"))
			}
			if !strings.Contains(err.Error(), "truncated") {
				t.Fatalf("error does not say the output was truncated: %v", err)
			}
		})
	}
}
