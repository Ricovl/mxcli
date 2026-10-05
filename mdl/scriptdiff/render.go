// SPDX-License-Identifier: Apache-2.0

package scriptdiff

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/executor"
)

// describeKinds maps a unit's storage $Type to the DESCRIBE that renders it.
// A unit type missing here is still reported — as written, with the paths that
// changed — just not rendered as MDL.
var describeKinds = map[string]ast.DescribeObjectType{
	"Microflows$Microflow":                           ast.DescribeMicroflow,
	"Microflows$Nanoflow":                            ast.DescribeNanoflow,
	"Microflows$Rule":                                ast.DescribeRule,
	"Forms$Page":                                     ast.DescribePage,
	"Forms$Snippet":                                  ast.DescribeSnippet,
	"Forms$Layout":                                   ast.DescribeLayout,
	"Forms$BuildingBlock":                            ast.DescribeBuildingBlock,
	"Enumerations$Enumeration":                       ast.DescribeEnumeration,
	"Constants$Constant":                             ast.DescribeConstant,
	"JavaActions$JavaAction":                         ast.DescribeJavaAction,
	"JavaScriptActions$JavaScriptAction":             ast.DescribeJavaScriptAction,
	"Workflows$Workflow":                             ast.DescribeWorkflow,
	"JsonStructures$JsonStructure":                   ast.DescribeJsonStructure,
	"ImportMappings$ImportMapping":                   ast.DescribeImportMapping,
	"ExportMappings$ExportMapping":                   ast.DescribeExportMapping,
	"Rest$ConsumedODataService":                      ast.DescribeODataClient,
	"ODataPublish$PublishedODataService2":            ast.DescribeODataService,
	"Rest$ConsumedRestService":                       ast.DescribeRestClient,
	"Rest$PublishedRestService":                      ast.DescribePublishedRestService,
	"Images$ImageCollection":                         ast.DescribeImageCollection,
	"CustomIcons$CustomIconCollection":               ast.DescribeIconCollection,
	"Menus$MenuDocument":                             ast.DescribeMenu,
	"ScheduledEvents$ScheduledEvent":                 ast.DescribeScheduledEvent,
	"Queues$Queue":                                   ast.DescribeQueue,
	"RegularExpressions$RegularExpression":           ast.DescribeRegularExpression,
	"BusinessEvents$BusinessEventService":            ast.DescribeBusinessEventService,
	"DatabaseConnector$DatabaseConnection":           ast.DescribeDatabaseConnection,
	"DataTransformers$DataTransformer":               ast.DescribeDataTransformer,
	"MessageDefinitions$MessageDefinitionCollection": ast.DescribeMessageDefinitionCollection,
	"MessageDefinitions$MessageDefinition2":          ast.DescribeMessageDefinition,
	"Navigation$NavigationDocument":                  ast.DescribeNavigation,
	"Settings$ProjectSettings":                       ast.DescribeSettings,
	"Projects$ModuleImpl":                            ast.DescribeModule,
}

// memberSpec is one list of separately described members in a unit that holds
// several: the entities of a domain model, the roles of module security.
type memberSpec struct {
	list, nameKey string
	kind          ast.DescribeObjectType
	qualified     bool // named Module.Name rather than Name
}

// memberDocs are the units rendered member by member, so a change reads as the
// member it is in, and the label of the unit when no member renders the change.
var memberDocs = map[string]struct {
	label   string
	members []memberSpec
}{
	"DomainModels$DomainModel": {"Domain model", []memberSpec{
		{"Entities", "Name", ast.DescribeEntity, true},
		{"Associations", "Name", ast.DescribeAssociation, true},
		{"CrossAssociations", "Name", ast.DescribeAssociation, true},
	}},
	"Security$ModuleSecurity": {"Module security", []memberSpec{
		{"ModuleRoles", "Name", ast.DescribeModuleRole, true},
	}},
	"Security$ProjectSecurity": {"Project security", []memberSpec{
		{"UserRoles", "Name", ast.DescribeUserRole, false},
		{"DemoUsers", "UserName", ast.DescribeDemoUser, false},
	}},
}

// partOfNewModule are the units `create module` adds along with the module
// itself; the module's own description covers them (its roles included), and
// a domain model is left out only while it is empty.
var partOfNewModule = map[string]bool{
	"Projects$ModuleSettings":  true,
	"Security$ModuleSecurity":  true,
	"DomainModels$DomainModel": true,
}

// renderer describes changed units on both sides: the project as it is, and the
// scratch copy exec wrote.
type renderer struct {
	before, after *side
}

type side struct {
	x    *executor.Executor
	out  *bytes.Buffer
	snap *Snapshot
}

func newRenderer(projectMpr, copyMpr string, before, after *Snapshot, opts Options) (*renderer, error) {
	open := func(path string, snap *Snapshot) (*side, error) {
		s := &side{out: &bytes.Buffer{}, snap: snap}
		s.x = newExecutor(s.out, opts)
		s.x.SetQuiet(true)
		if err := s.x.Execute(&ast.ConnectStmt{Path: path}); err != nil {
			s.x.Close()
			return nil, fmt.Errorf("connect to %s to describe it: %w", path, err)
		}
		return s, nil
	}
	b, err := open(projectMpr, before)
	if err != nil {
		return nil, err
	}
	a, err := open(copyMpr, after)
	if err != nil {
		b.close()
		return nil, err
	}
	return &renderer{before: b, after: a}, nil
}

func (r *renderer) close() {
	r.before.close()
	r.after.close()
}

func (s *side) close() {
	_ = s.x.Execute(&ast.DisconnectStmt{})
	s.x.Close()
}

// describe renders one element, without describe's language header line.
func (s *side) describe(kind ast.DescribeObjectType, name ast.QualifiedName) (string, error) {
	s.out.Reset()
	if err := s.x.Execute(&ast.DescribeStmt{ObjectType: kind, Name: name}); err != nil {
		return "", err
	}
	text := strings.TrimRight(s.out.String(), "\n")
	if first, rest, ok := strings.Cut(text, "\n"); ok && strings.HasPrefix(first, "mdl ") && strings.HasSuffix(first, ";") {
		text = rest
	}
	return text, nil
}

func (r *renderer) render(units []UnitChange) []executor.DiffResult {
	newModules := map[string]bool{}
	for _, u := range units {
		if u.Kind == Added && u.Type == "Projects$ModuleImpl" {
			newModules[u.Name] = true
		}
	}
	var out []executor.DiffResult
	for _, u := range units {
		if u.Kind == Added && partOfNewModule[u.Type] && newModules[u.Name] &&
			(u.Type != "DomainModels$DomainModel" || len(members(u.Type, u.after)) == 0) {
			continue
		}
		if _, ok := memberDocs[u.Type]; ok {
			out = append(out, r.members(u)...)
			continue
		}
		out = append(out, r.document(u))
	}
	return out
}

// document renders one unit that is one document.
func (r *renderer) document(u UnitChange) executor.DiffResult {
	name := u.Name
	if name == "" {
		name = typeLabel(u.Type) // a project-level document: navigation, settings
	}
	res := executor.DiffResult{
		ObjectType: typeLabel(u.Type),
		ObjectName: splitName(name),
		IsNew:      u.Kind == Added,
		IsDeleted:  u.Kind == Removed,
	}
	kind, ok := describeKinds[u.Type]
	var errs []string
	target := res.ObjectName
	if kind == ast.DescribeModule {
		target = ast.QualifiedName{Module: u.Name} // DESCRIBE MODULE names it so
	}
	if ok {
		res.ObjectType = labelOf(kind)
		if u.Kind != Added {
			text, err := r.before.describe(kind, target)
			if err != nil {
				errs = append(errs, "current: "+err.Error())
			}
			res.Current = text
		}
		if u.Kind != Removed {
			text, err := r.after.describe(kind, target)
			if err != nil {
				errs = append(errs, "after exec: "+err.Error())
			}
			res.Proposed = text
		}
	}
	res.Writes = writesNote(u, res.Current == res.Proposed, ok && len(errs) == 0, errs)
	if u.Type == "Projects$Folder" {
		// A folder is its path, which the name already says.
		res.Writes = map[ChangeKind]string{Added: "a new folder", Removed: "folder removed"}[u.Kind]
		if u.Kind == Modified {
			res.Writes = writesNote(u, true, true, nil)
		}
	}
	res.Changes = executor.LineChanges(res.Current, res.Proposed)
	return res
}

// members renders a unit that holds several described members — a domain
// model's entities and associations, a module's roles, the project's user
// roles and demo users — one result per member that changes.
func (r *renderer) members(u UnitChange) []executor.DiffResult {
	doc := memberDocs[u.Type]
	mod := u.Name
	seen := map[member]bool{}
	var all []member
	for _, raw := range [][]byte{u.before, u.after} {
		for _, m := range members(u.Type, raw) {
			if !seen[m] {
				seen[m] = true
				all = append(all, m)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].kind != all[j].kind {
			return all[i].kind < all[j].kind
		}
		return all[i].name < all[j].name
	})
	beforeHas := memberSet(u.Type, u.before)
	afterHas := memberSet(u.Type, u.after)

	var out []executor.DiffResult
	for _, m := range all {
		qn := ast.QualifiedName{Name: m.name}
		if m.qualified {
			qn.Module = mod
		}
		res := executor.DiffResult{ObjectType: labelOf(m.kind), ObjectName: qn}
		var errs []string
		if beforeHas[m] {
			text, err := r.before.describe(m.kind, qn)
			if err != nil {
				errs = append(errs, "current: "+err.Error())
			}
			res.Current = text
		} else {
			res.IsNew = true
		}
		if afterHas[m] {
			text, err := r.after.describe(m.kind, qn)
			if err != nil {
				errs = append(errs, "after exec: "+err.Error())
			}
			res.Proposed = text
		} else {
			res.IsDeleted = true
		}
		if len(errs) > 0 {
			res.Writes = "could not be described: " + strings.Join(errs, "; ")
		} else if res.Current == res.Proposed && !res.IsNew && !res.IsDeleted {
			continue
		}
		res.Changes = executor.LineChanges(res.Current, res.Proposed)
		out = append(out, res)
	}
	if len(out) == 0 {
		// The unit is written, but no member describes differently: say what
		// changed rather than nothing.
		res := executor.DiffResult{
			ObjectType: doc.label,
			ObjectName: ast.QualifiedName{Name: mod},
			IsNew:      u.Kind == Added,
			IsDeleted:  u.Kind == Removed,
			Writes:     writesNote(u, true, true, nil),
		}
		switch u.Kind {
		case Added:
			res.Writes = "created empty"
		case Removed:
			res.Writes = "removed"
		}
		out = append(out, res)
	}
	return out
}

// writesNote says what exec writes when the rendering does not show it, or
// cannot be made; "" when the rendering shows the change.
func writesNote(u UnitChange, renderedSame, described bool, errs []string) string {
	var what []string
	if len(errs) > 0 {
		what = append(what, "could not be described: "+strings.Join(errs, "; "))
	}
	if u.Moved != "" {
		what = append(what, fmt.Sprintf("moved to '%s'", u.Moved))
	}
	if u.Kind == Modified && u.Rewritten && (renderedSame || !described) {
		switch {
		case u.IDChurnOnly:
			what = append(what, "rewritten with only element $IDs changed (canonically equal)")
		default:
			if paths := changedPaths(u.before, u.after, 4); len(paths) > 0 {
				what = append(what, "changed: "+strings.Join(paths, ", "))
			} else {
				what = append(what, "rewritten")
			}
		}
	}
	if (u.Kind == Added || u.Kind == Removed) && !described && len(errs) == 0 {
		what = append(what, "no MDL rendering for "+u.Type)
	}
	return strings.Join(what, "; ")
}

type member struct {
	kind      ast.DescribeObjectType
	name      string
	qualified bool
}

// members lists the described members of a unit of a type in memberDocs.
func members(typ string, raw []byte) []member {
	if len(raw) == 0 {
		return nil
	}
	doc := bson.Raw(raw)
	var out []member
	for _, spec := range memberDocs[typ].members {
		arr, ok := doc.Lookup(spec.list).ArrayOK()
		if !ok {
			continue
		}
		vals, _ := arr.Values()
		for _, v := range vals {
			d, ok := v.DocumentOK()
			if !ok {
				continue
			}
			if name, ok := d.Lookup(spec.nameKey).StringValueOK(); ok && name != "" {
				out = append(out, member{spec.kind, name, spec.qualified})
			}
		}
	}
	return out
}

func memberSet(typ string, raw []byte) map[member]bool {
	m := map[member]bool{}
	for _, e := range members(typ, raw) {
		m[e] = true
	}
	return m
}

// changedPaths lists up to max property paths whose values differ between two
// units, element $IDs aside. A list element is named by its Name where it has
// one.
func changedPaths(a, b []byte, max int) []string {
	var out []string
	var walk func(path string, x, y bson.RawValue)
	walk = func(path string, x, y bson.RawValue) {
		if len(out) >= max {
			return
		}
		if x.Type != y.Type {
			out = append(out, path)
			return
		}
		switch x.Type {
		case bson.TypeEmbeddedDocument:
			dx, dy := x.Document(), y.Document()
			ex, _ := dx.Elements()
			keys := map[string]bool{}
			for _, e := range ex {
				k := e.Key()
				keys[k] = true
				if k == "$ID" {
					continue
				}
				vy, err := dy.LookupErr(k)
				if err != nil {
					out = append(out, join(path, k))
				} else {
					walk(join(path, k), e.Value(), vy)
				}
				if len(out) >= max {
					return
				}
			}
			ey, _ := dy.Elements()
			for _, e := range ey {
				if !keys[e.Key()] && len(out) < max {
					out = append(out, join(path, e.Key()))
				}
			}
		case bson.TypeArray:
			vx, _ := x.Array().Values()
			vy, _ := y.Array().Values()
			if len(vx) != len(vy) {
				out = append(out, fmt.Sprintf("%s (%d -> %d items)", path, len(vx), len(vy)))
				return
			}
			for i := range vx {
				walk(fmt.Sprintf("%s[%s]", path, itemName(vx[i], i)), vx[i], vy[i])
				if len(out) >= max {
					return
				}
			}
		case bson.TypeBinary:
			// A pointer holds an element $ID, so a pointer to an element exec
			// re-minted differs without anything having changed; canon has
			// already said whether the unit did. A GUID is reported.
			if !bytes.Equal(x.Value, y.Value) && !strings.HasSuffix(path, "Pointer") {
				out = append(out, path)
			}
		default:
			if !bytes.Equal(x.Value, y.Value) {
				out = append(out, path)
			}
		}
	}
	walk("", bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: a}, bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: b})
	return out
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func itemName(v bson.RawValue, i int) string {
	if d, ok := v.DocumentOK(); ok {
		if n, ok := d.Lookup("Name").StringValueOK(); ok && n != "" {
			return n
		}
	}
	return fmt.Sprint(i)
}

// labelOf turns a describe kind into a label: "Java Action".
func labelOf(k ast.DescribeObjectType) string {
	words := strings.Fields(strings.ToLower(k.String()))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// typeLabel turns a storage type into a label for a unit with no describe:
// "Security$ModuleSecurity" -> "ModuleSecurity".
func typeLabel(t string) string {
	if _, after, ok := strings.Cut(t, "$"); ok {
		return after
	}
	return t
}

func splitName(q string) ast.QualifiedName {
	if mod, name, ok := strings.Cut(q, "."); ok {
		return ast.QualifiedName{Module: mod, Name: name}
	}
	return ast.QualifiedName{Name: q}
}
