// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A property list whose key is unknown, or whose value has a shape its key
// does not take, parses under mdl 0 as it always has — the property is
// ignored or read by its shape — and warns; under mdl 1 it is an error that
// names the key and, for a misspelling, the key meant (#732, ADR-0010 R11).
var strictPropertyCases = []struct {
	name string
	src  string
	code string
	// what the mdl 1 error must say
	want []string
}{
	{
		"rest client operation key",
		"create rest client M.Api (BaseUrl: 'https://x', Authentication: none) { operation GetUser { Method: get, pathh: '/users', Response: none } };",
		"MDL-V1-PROP", []string{"unknown property 'pathh'", "REST client operation", "did you mean 'Path'"},
	},
	{
		"rest client key",
		"create rest client M.Api (BaseUrll: 'https://x', Authentication: none) { operation GetUser { Method: get, Path: '/u', Response: none } };",
		"MDL-V1-PROP", []string{"unknown property 'BaseUrll'", "did you mean 'BaseUrl'"},
	},
	{
		"rest client basic auth key",
		"create rest client M.Api (BaseUrl: 'https://x', Authentication: basic (Username: 'u', Passwd: 'p')) { operation GetUser { Method: get, Path: '/u', Response: none } };",
		"MDL-V1-PROP", []string{"unknown property 'Passwd'", "did you mean 'Password'"},
	},
	{
		"response written as a body",
		"create rest client M.Api (BaseUrl: 'https://x', Authentication: none) { operation Post { Method: post, Path: '/u', Response: json from $X } };",
		"MDL-V1-PROPVALUE", []string{"'Response'", "json as $var"},
	},
	{
		"body written as a response",
		"create rest client M.Api (BaseUrl: 'https://x', Authentication: none) { operation Post { Method: post, Path: '/u', Body: status as $Y, Response: none } };",
		"MDL-V1-PROPVALUE", []string{"'Body'", "json from $var"},
	},
	{
		"rest client path as a number",
		"create rest client M.Api (BaseUrl: 'https://x', Authentication: none) { operation Get { Method: get, Path: 42, Response: none } };",
		"MDL-V1-PROPVALUE", []string{"'Path'", "a string"},
	},
	{
		"published rest key",
		"create published rest service M.S (Path: 'rest/s', Verison: '1.0') { };",
		"MDL-V1-PROP", []string{"unknown property 'Verison'", "published REST service", "did you mean 'Version'"},
	},
	{
		"business event service key",
		"create business event service M.BE (ServiceNme: 'S', EventNamePrefix: '') { message Changed (Id: Long) publish; };",
		"MDL-V1-PROP", []string{"unknown property 'ServiceNme'", "did you mean 'ServiceName'"},
	},
	{
		"model key",
		"create model M.GPT (Provider: MxCloudGenAI, Key: M.Key, DisplayNmae: 'GPT');",
		"MDL-V1-PROP", []string{"unknown property 'DisplayNmae'", "model", "did you mean 'DisplayName'"},
	},
	{
		"model provider as a string",
		"create model M.GPT (Provider: 'MxCloudGenAI', Key: M.Key);",
		"MDL-V1-PROPVALUE", []string{"'Provider'", "a name"},
	},
	{
		"knowledge base key",
		"create knowledge base M.KB (Provider: MxCloudGenAI, Key: M.Key, Enviroment: 'x');",
		"MDL-V1-PROP", []string{"unknown property 'Enviroment'", "did you mean 'Environment'"},
	},
	{
		"consumed mcp service key",
		"create consumed mcp service M.Mcp (ProtocolVersion: v2025_03_26, Version: '1', Timeout: 30);",
		"MDL-V1-PROP", []string{"unknown property 'Timeout'", "did you mean 'ConnectionTimeoutSeconds'"},
	},
	{
		"agent key",
		"create agent M.A (UsageType: Task, Model: M.GPT, Temprature: 0.5);",
		"MDL-V1-PROP", []string{"unknown property 'Temprature'", "agent", "did you mean 'Temperature'"},
	},
	{
		"agent model as a name",
		"create agent M.A (UsageType: Task, Model: GPT);",
		"MDL-V1-PROPVALUE", []string{"'Model'", "a qualified name"},
	},
	{
		"agent tool block key",
		"create agent M.A (UsageType: Task, Model: M.GPT) { tool T { Enabeld: true } };",
		"MDL-V1-PROP", []string{"unknown property 'Enabeld'", "tool", "did you mean 'Enabled'"},
	},
}

func TestStrictPropertiesByVersion(t *testing.T) {
	for _, tc := range strictPropertyCases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("mdl 0 must keep accepting it: %v", errs)
			}
			var notes []ast.LanguageNote
			for _, n := range prog.LanguageNotes {
				if strings.HasPrefix(n.Code, "MDL-V1-PROP") {
					notes = append(notes, n)
				}
			}
			if len(notes) != 1 || notes[0].Code != tc.code || notes[0].Line != 1 {
				t.Fatalf("mdl 0: want one %s warning on line 1, got %+v", tc.code, notes)
			}

			_, errs = Build("mdl 1;\n" + tc.src)
			if len(errs) != 1 {
				t.Fatalf("mdl 1: want one error, got %v", errs)
			}
			for _, w := range tc.want {
				if !strings.Contains(errs[0].Error(), w) {
					t.Errorf("mdl 1: the error does not say %q:\n%v", w, errs[0])
				}
			}
			if !strings.HasPrefix(errs[0].Error(), "line 2:") {
				t.Errorf("mdl 1: the error must name the line: %v", errs[0])
			}
		})
	}
}

// Control: the forms describe writes, and every key each list reads, are
// accepted under mdl 1 without an error or a warning.
func TestStrictPropertiesAcceptWhatDescribeWrites(t *testing.T) {
	for _, src := range []string{
		`create rest client M.Api (
  BaseUrl: 'https://x',
  Authentication: basic (Username: @M.User, Password: $Pwd),
  Folder: 'Integrations'
)
{
  operation GetUser {
    Method: get,
    Path: '/users/{id}',
    Parameters: ($id: String),
    Query: ($q: String),
    Headers: ('Accept' = 'application/json', 'Auth' = 'Bearer ' + $Token),
    Body: json from $Payload,
    Timeout: 30,
    Response: json as $Result
  }
  operation Put { Method: put, Path: '/x', Body: file from $F, Response: status as $S }
  operation Tpl { Method: post, Path: '/x', Body: template '{}', Response: string as $S }
  operation Map { Method: post, Path: '/x', Body: mapping M.Req { Name = name }, Response: mapping M.Res { Name = name } }
  operation Del { Method: delete, Path: '/x', Response: file as $F }
};`,
		"create rest client M.Open (OpenApi: 'spec.json', BaseUrl: 'https://x', Authentication: none);",
		"create published rest service M.S (Path: 'rest/s', Version: '1.0', ServiceName: 'S', Folder: 'F') { };",
		"create business event service M.BE (ServiceName: 'S', EventNamePrefix: '', Folder: 'F') { message Changed (Id: Long) publish; };",
		"create model M.GPT (Provider: MxCloudGenAI, Key: M.Key, DisplayName: 'GPT', KeyName: 'k', KeyId: 'i', Environment: 'e', ResourceName: 'r', DeepLinkURL: 'u');",
		"create knowledge base M.KB (Provider: MxCloudGenAI, Key: M.Key, ModelDisplayName: 'd', ModelName: 'n', KeyName: 'k', KeyId: 'i', Environment: 'e', DeepLinkURL: 'u');",
		"create consumed mcp service M.Mcp (ProtocolVersion: 'v2025_03_26', Version: '1', ConnectionTimeoutSeconds: 30, Documentation: 'd');",
		`create agent M.A (
  UsageType: Task,
  Description: 'd',
  Model: M.GPT,
  Entity: M.Ctx,
  Variables: ("Name": EntityAttribute, "Other": String),
  MaxTokens: 100,
  ToolChoice: Auto,
  Temperature: 0.5,
  TopP: 1,
  SystemPrompt: $$You help.$$,
  UserPrompt: 'Hi'
)
{
  mcp service M.Mcp {
    Enabled: true,
    Description: 'd'
  }
  tool T {
    ToolType: Microflow,
    Document: M.DoIt,
    Enabled: false,
    Description: 'd'
  }
  knowledge base KB {
    Source: M.KB,
    Collection: 'c',
    MaxResults: 5,
    Description: 'd',
    Enabled: true
  }
};`,
	} {
		prog, errs := Build("mdl 1;\n" + src)
		if len(errs) > 0 {
			t.Errorf("mdl 1 refused a well-formed property list: %v\n%s", errs, src)
			continue
		}
		for _, n := range prog.LanguageNotes {
			t.Errorf("mdl 1 warned on a well-formed property list: %+v", n)
		}
		prog, _ = Build(src)
		for _, n := range prog.LanguageNotes {
			if strings.HasPrefix(n.Code, "MDL-V1-PROP") {
				t.Errorf("mdl 0 warned on a well-formed property list: %+v", n)
			}
		}
	}
}

// Under mdl 0 a mis-shaped value keeps its old reading: `Response: json from
// $X` sets the request body, as it always has.
func TestStrictPropertiesKeepTheOldReadingUnderMdl0(t *testing.T) {
	prog, errs := Build(strictPropertyCases[3].src)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	op := prog.Statements[0].(*ast.CreateRestClientStmt).Operations[0]
	if op.BodyType != "json" || op.BodyVariable != "$X" || op.ResponseType != "" {
		t.Fatalf("mdl 0 changed what `Response: json from $X` builds: %+v", op)
	}
}
