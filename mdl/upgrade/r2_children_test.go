// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R2 (ako/mxcli#754): the integration documents' old brackets are rewritten to
// properties in ( ) and children in { }, every pair in a statement at once,
// with the body mappings, headers and comments left as written.
func TestUpgrade_R2IntegrationDocumentBrackets(t *testing.T) {
	src := `create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none)
{
  /** Fetch one user */
  operation GetUser {
    Method: get,
    Path: '/u/{id}',
    Headers: ('Accept' = 'application/json'),
    Response: mapping M.User { Name = name }
  }
  operation Ping { Method: get, Path: '/ping', Response: none }
};
CREATE AGENT M.A (UsageType: Task, Model: M.Gpt, SystemPrompt: 'x') {
  TOOL Lookup { Description: 'Find' } -- the lookup tool
};
alter agent M.A add knowledge base Docs { Source: M.KB };
create image collection M.Icons export level 'Public' (
  image Logo from file 'logo.png',
  image Home from file 'home.png'
);
create message definition collection M.Msgs (
  definition Order for M.Order (
    Number,
    M.Order_Line/M.Line as 'Lines' ( Sku )
  )
);
alter message definition M.Msgs.Order add member M.Order_Tag/M.Tag ( Label );
ALTER NANOFLOW M.N {
  INSERT AFTER $X { LOG INFO 'x'; }
  replace commit $O with {
    commit $O with events;
  };
};
`
	want := `create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none)
{
  /** Fetch one user */
  operation GetUser (
    Method: get,
    Path: '/u/{id}',
    Headers: ('Accept' = 'application/json'),
    Response: mapping M.User { Name = name }
  )
  operation Ping ( Method: get, Path: '/ping', Response: none )
};
CREATE AGENT M.A (UsageType: Task, Model: M.Gpt, SystemPrompt: 'x') {
  TOOL Lookup ( Description: 'Find' ) -- the lookup tool
};
alter agent M.A add knowledge base Docs ( Source: M.KB );
create image collection M.Icons export level 'Public' {
  image Logo ( File: 'logo.png' )
  image Home ( File: 'home.png' )
};
create message definition collection M.Msgs {
  definition Order for M.Order {
    Number,
    M.Order_Line/M.Line as 'Lines' { Sku }
  }
};
alter message definition M.Msgs.Order add member M.Order_Tag/M.Tag { Label };
ALTER NANOFLOW M.N {
  INSERT AFTER $X BEGIN LOG INFO 'x'; END
  replace commit $O with begin
    commit $O with events;
  end;
};
`
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	for code, n := range map[string]int{
		deprecation.RestOperationBraces: 1, deprecation.AgentAttachmentBraces: 2,
		deprecation.ImageCollectionParens: 1, deprecation.MessageTreeParens: 2,
		deprecation.AlterFlowFragmentBraces: 1,
	} {
		if res.Rewritten[code] != n {
			t.Errorf("Rewritten[%s] = %d, want %d (all: %v)", code, res.Rewritten[code], n, res.Rewritten)
		}
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}
