// SPDX-License-Identifier: Apache-2.0

package executor

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
)

// The per-kind property sets in widget_native_props.go are only as good as
// their agreement with the builders. A set that is missing a key the builder
// reads refuses a property that IS written; a set holding a key nothing reads
// accepts one that is dropped — the bug class itself. So the sets are checked
// against the source, not against a second hand-typed list (the lesson of
// mendixlabs/mxcli#1346, where the guard drifted with the list it guarded).

// builderSource is the executor package's non-test source, indexed for a
// taint walk: which keys does a function read off the *ast.WidgetV3 it is
// given, directly or through the functions it hands that widget to?
type builderSource struct {
	fset  *token.FileSet
	funcs map[string]*builderFunc // "pageBuilder.buildTextBoxV3", "inputLabel"
	names map[string][]string     // bare name -> keys of funcs
}

type builderFunc struct {
	decl    *goast.FuncDecl
	widgets map[int]string // index of each *ast.WidgetV3 parameter -> its name
	keyArgs map[int]bool   // string parameters the function uses as a property key
}

// widgetGetterKeys are ast.WidgetV3's typed getters and the key each reads.
var widgetGetterKeys = map[string]string{
	"GetDataSource": "DataSource", "GetAction": "Action", "GetOnChange": "OnChange", "GetPlaceholder": "Placeholder",
	"GetAttribute": "Attribute", "GetBinds": "Binds", "GetLabel": "Label", "GetCaption": "Caption", "GetContent": "Content",
	"GetRenderMode": "RenderMode", "GetButtonStyle": "ButtonStyle", "GetClass": "Class", "GetStyle": "Style",
	"GetDynamicClasses": "DynamicClasses", "GetDesktopWidth": "DesktopWidth", "GetContentParams": "ContentParams",
	"GetCaptionParams": "CaptionParams", "GetAttributes": "Attributes", "GetFilterType": "FilterType", "GetAttr": "Attr",
	"GetSnippet": "Snippet", "GetSnippetParams": "Params", "GetSelection": "Selection", "GetDesignProperties": "DesignProperties",
}

// messageOnlyReaders read a key to NAME the widget in an error, not to write
// it, so their reads are not consumption. widgetRefForMessage falls back to
// the Caption when a widget has no name — which, counted, made every input
// "read" Caption, the very key #1698 is about.
// refuseNativeWidgetProps is the verdict itself: it reads every key to judge
// it, not to write it.
var messageOnlyReaders = map[string]bool{"widgetRefForMessage": true, "refuseNativeWidgetProps": true}

// tableDrivenReaders read keys from a per-type table rather than by literal;
// nativeWidgetAccepts consults the same table (inputFormatProp).
var tableDrivenReaders = map[string]bool{"inputFormattingInfo": true, "inputFormattingProblems": true}

func loadBuilderSource(t *testing.T) *builderSource {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	src := &builderSource{fset: token.NewFileSet(), funcs: map[string]*builderFunc{}, names: map[string][]string{}}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(src.fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*goast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			bf := &builderFunc{decl: fd, widgets: map[int]string{}, keyArgs: map[int]bool{}}
			strParams := map[string]int{}
			i := 0
			for _, field := range fd.Type.Params.List {
				for _, n := range field.Names {
					if isWidgetV3Type(field.Type) {
						bf.widgets[i] = n.Name
					}
					if id, ok := field.Type.(*goast.Ident); ok && id.Name == "string" {
						strParams[n.Name] = i
					}
					i++
				}
				if len(field.Names) == 0 {
					i++
				}
			}
			if len(bf.widgets) == 0 {
				continue
			}
			goast.Inspect(fd.Body, func(n goast.Node) bool {
				switch x := n.(type) {
				case *goast.IndexExpr:
					if sel, ok := x.X.(*goast.SelectorExpr); ok && sel.Sel.Name == "Properties" {
						if id, ok := x.Index.(*goast.Ident); ok {
							if j, ok := strParams[id.Name]; ok {
								bf.keyArgs[j] = true
							}
						}
					}
				case *goast.CallExpr:
					name := calleeName(x)
					if !isKeyAccessor(name) {
						break
					}
					for _, a := range x.Args {
						if id, ok := a.(*goast.Ident); ok {
							if j, ok := strParams[id.Name]; ok {
								bf.keyArgs[j] = true
							}
						}
					}
				}
				return true
			})
			key := fd.Name.Name
			if r := receiverTypeName(fd); r != "" {
				key = r + "." + key
			}
			src.funcs[key] = bf
			src.names[fd.Name.Name] = append(src.names[fd.Name.Name], key)
		}
	}
	return src
}

func isWidgetV3Type(e goast.Expr) bool {
	st, ok := e.(*goast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := st.X.(*goast.SelectorExpr)
	return ok && sel.Sel.Name == "WidgetV3"
}

func receiverTypeName(fd *goast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	t := fd.Recv.List[0].Type
	if st, ok := t.(*goast.StarExpr); ok {
		t = st.X
	}
	if id, ok := t.(*goast.Ident); ok {
		return id.Name
	}
	return ""
}

func calleeName(c *goast.CallExpr) string {
	switch f := c.Fun.(type) {
	case *goast.Ident:
		return f.Name
	case *goast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func isKeyAccessor(name string) bool {
	return (strings.HasPrefix(name, "Get") && strings.HasSuffix(name, "Prop")) || name == "lookupPropCI" || name == "ExpressionSource"
}

func stringLit(e goast.Expr) (string, bool) {
	bl, ok := e.(*goast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	return s, err == nil
}

// builderReads is what one taint walk found.
type builderReads struct {
	keys    map[string]bool
	dynamic map[string]bool // functions reading a key that is not a literal
	unknown map[string]bool // calls passing the widget to a function not found
}

// reads walks fn from its widget parameter(s).
func (s *builderSource) reads(fn string) builderReads {
	r := builderReads{keys: map[string]bool{}, dynamic: map[string]bool{}, unknown: map[string]bool{}}
	visited := map[string]bool{}
	bf := s.funcs[fn]
	if bf == nil {
		r.unknown[fn] = true
		return r
	}
	for idx := range bf.widgets {
		s.walk(fn, idx, &r, visited)
	}
	return r
}

func (s *builderSource) walk(fn string, idx int, r *builderReads, visited map[string]bool) {
	vk := fn + "#" + strconv.Itoa(idx)
	if visited[vk] {
		return
	}
	visited[vk] = true
	bf := s.funcs[fn]
	if bf == nil || messageOnlyReaders[bf.decl.Name.Name] {
		return
	}
	if tableDrivenReaders[bf.decl.Name.Name] {
		r.dynamic[bf.decl.Name.Name] = true
		return
	}
	w := map[string]bool{bf.widgets[idx]: true}
	recv := ""
	if bf.decl.Recv != nil && len(bf.decl.Recv.List) > 0 && len(bf.decl.Recv.List[0].Names) > 0 {
		recv = bf.decl.Recv.List[0].Names[0].Name
	}
	// `for _, key := range []string{"Icon", "icon"}` — literal keys by variable.
	litVars := map[string][]string{}
	goast.Inspect(bf.decl.Body, func(n goast.Node) bool {
		if as, ok := n.(*goast.AssignStmt); ok && len(as.Lhs) == 1 && len(as.Rhs) == 1 {
			if id, ok := as.Rhs[0].(*goast.Ident); ok && w[id.Name] {
				if l, ok := as.Lhs[0].(*goast.Ident); ok {
					w[l.Name] = true
				}
			}
		}
		if rs, ok := n.(*goast.RangeStmt); ok {
			if cl, ok := rs.X.(*goast.CompositeLit); ok {
				if v, ok := rs.Value.(*goast.Ident); ok {
					for _, e := range cl.Elts {
						if lit, ok := stringLit(e); ok {
							litVars[v.Name] = append(litVars[v.Name], lit)
						}
					}
				}
			}
		}
		return true
	})
	isW := func(e goast.Expr) bool {
		id, ok := e.(*goast.Ident)
		return ok && w[id.Name]
	}
	keyOf := func(e goast.Expr) ([]string, bool) {
		if lit, ok := stringLit(e); ok {
			return []string{lit}, true
		}
		if id, ok := e.(*goast.Ident); ok && len(litVars[id.Name]) > 0 {
			return litVars[id.Name], true
		}
		return nil, false
	}
	goast.Inspect(bf.decl.Body, func(n goast.Node) bool {
		switch x := n.(type) {
		case *goast.RangeStmt:
			if sel, ok := x.X.(*goast.SelectorExpr); ok && sel.Sel.Name == "Properties" && isW(sel.X) {
				r.dynamic[fn] = true
			}
		case *goast.IndexExpr:
			if sel, ok := x.X.(*goast.SelectorExpr); ok && sel.Sel.Name == "Properties" && isW(sel.X) {
				if ks, ok := keyOf(x.Index); ok {
					for _, k := range ks {
						r.keys[k] = true
					}
				} else if len(bf.keyArgs) == 0 {
					r.dynamic[fn] = true
				}
			}
		case *goast.CallExpr:
			if sel, ok := x.Fun.(*goast.SelectorExpr); ok && isW(sel.X) {
				if k, ok := widgetGetterKeys[sel.Sel.Name]; ok {
					r.keys[k] = true
				}
				if isKeyAccessor(sel.Sel.Name) && len(x.Args) >= 1 {
					if ks, ok := keyOf(x.Args[0]); ok {
						for _, k := range ks {
							r.keys[k] = true
						}
					} else if len(bf.keyArgs) == 0 {
						r.dynamic[fn] = true
					}
				}
			}
			for i, a := range x.Args {
				if !isW(a) {
					continue
				}
				found := false
				for _, target := range s.resolve(x, recv) {
					tf := s.funcs[target]
					if tf == nil {
						continue
					}
					if _, ok := tf.widgets[i]; !ok {
						continue
					}
					found = true
					if len(tf.keyArgs) > 0 {
						// A key accessor: the read is the key this call passes.
						for j, b := range x.Args {
							if !tf.keyArgs[j] {
								continue
							}
							if ks, ok := keyOf(b); ok {
								for _, k := range ks {
									r.keys[k] = true
								}
							} else {
								r.dynamic[fn] = true
							}
						}
						continue
					}
					s.walk(target, i, r, visited)
				}
				if !found {
					r.unknown[calleeName(x)] = true
				}
			}
		}
		return true
	})
}

func (s *builderSource) resolve(c *goast.CallExpr, recv string) []string {
	switch f := c.Fun.(type) {
	case *goast.Ident:
		if _, ok := s.funcs[f.Name]; ok {
			return []string{f.Name}
		}
	case *goast.SelectorExpr:
		if id, ok := f.X.(*goast.Ident); ok && recv != "" && id.Name == recv {
			for _, k := range s.names[f.Sel.Name] {
				if strings.Contains(k, ".") {
					return []string{k}
				}
			}
		}
		return s.names[f.Sel.Name]
	}
	return nil
}

// dispatch reads buildWidgetV3's switch: keyword -> the builder it calls.
func (s *builderSource) dispatch(t *testing.T) map[string]string {
	t.Helper()
	bw := s.funcs["pageBuilder.buildWidgetV3"]
	if bw == nil {
		t.Fatal("pageBuilder.buildWidgetV3 not found")
	}
	out := map[string]string{}
	goast.Inspect(bw.decl.Body, func(n goast.Node) bool {
		cc, ok := n.(*goast.CaseClause)
		if !ok {
			return true
		}
		callee := ""
		for _, stmt := range cc.Body {
			goast.Inspect(stmt, func(m goast.Node) bool {
				if c, ok := m.(*goast.CallExpr); ok && callee == "" {
					if sel, ok := c.Fun.(*goast.SelectorExpr); ok && strings.HasPrefix(sel.Sel.Name, "build") {
						callee = "pageBuilder." + sel.Sel.Name
					}
				}
				return true
			})
		}
		for _, e := range cc.List {
			if lit, ok := stringLit(e); ok {
				out[lit] = callee
			}
		}
		return true
	})
	if len(out) < 30 {
		t.Fatalf("found only %d keywords in buildWidgetV3's switch — the scan no longer matches its shape", len(out))
	}
	return out
}

// nativeChildBuilders are the builders of the elements a parent reads itself.
// "layoutheader region" and "dataview footer" are read inline by their parent
// (only their children are used), so they have none and take nothing.
var nativeChildBuilders = map[string]string{
	"scrollcontainer region": "pageBuilder.buildScrollContainerRegionV3",
	"layoutgrid row":         "pageBuilder.buildLayoutGridRowV3",
	"layoutgrid column":      "pageBuilder.buildLayoutGridColumnV3",
	"navigationlist item":    "pageBuilder.buildNavigationListItemV3",
	"listview template":      "pageBuilder.buildListViewTemplateV3",
}

// dispatchedWithoutKind are buildWidgetV3 keywords with no property set, and why.
var dispatchedWithoutKind = map[string]string{
	"text":           "refused outright: Forms$Text is not a Mendix type",
	"statictext":     "refused outright: Forms$Text is not a Mendix type",
	"image":          "the pluggable image widget; the static fallback is used only with no engine",
	"legacydatagrid": "refused outright: no writer",
	"tabpage":        "refused outside a tab container; judged there by MDL-WIDGET44 (#1465)",
	"region":         "refused outside a scroll container or layout header; judged there",
	"item":           "refused outside a navigation list; judged there",
	"slot":           "refused outside a fragment body",
}

func lowerSet(keys []string) map[string]bool {
	m := map[string]bool{}
	for _, k := range keys {
		m[strings.ToLower(k)] = true
	}
	return m
}

func sortedNameSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Every key a kind's builder reads is in its `reads`, and every key in its
// `reads` is read by the builder — exactly, both ways.
func TestNativeWidgetProps_MatchBuilderReads(t *testing.T) {
	src := loadBuilderSource(t)
	dispatch := src.dispatch(t)

	for keyword, fn := range dispatch {
		if _, ok := nativeWidgetKinds[keyword]; !ok && dispatchedWithoutKind[keyword] == "" {
			t.Errorf("buildWidgetV3 builds `%s` (%s) but nativeWidgetKinds has no entry for it — add its "+
				"property set, or list it in dispatchedWithoutKind with the reason", keyword, fn)
		}
	}

	for kind, k := range nativeWidgetKinds {
		fn := nativeChildBuilders[kind]
		if !strings.Contains(kind, " ") {
			fn = dispatch[kind]
		}
		want := lowerSet(k.reads)
		if fn == "" {
			if len(want) > 0 {
				t.Errorf("%s: no builder found, but reads lists %v", kind, k.reads)
			}
			continue
		}
		r := src.reads(fn)
		if len(r.unknown) > 0 {
			t.Errorf("%s (%s): passes the widget to functions the scan cannot resolve: %v", kind, fn, sortedNameSet(r.unknown))
		}
		for d := range r.dynamic {
			if !tableDrivenReaders[d] {
				t.Errorf("%s (%s): %s reads a key that is not a literal — make it one, or add it to "+
					"tableDrivenReaders with the predicate nativeWidgetAccepts uses for it", kind, fn, d)
			}
		}
		got := map[string]bool{}
		for key := range r.keys {
			got[strings.ToLower(key)] = true
		}
		for key := range got {
			if !want[key] {
				t.Errorf("%s: %s reads `%s`, which nativeWidgetKinds[%q].reads lacks — check and exec would "+
					"refuse a property that is written", kind, fn, key, kind)
			}
		}
		for key := range want {
			if !got[key] {
				t.Errorf("%s: nativeWidgetKinds[%q].reads has `%s`, which %s never reads — it would be "+
					"accepted and dropped", kind, kind, key, fn)
			}
		}
	}

	// The common tail: exactly appearance, visibility and editability.
	tail := map[string]bool{}
	for _, fn := range []string{"applyWidgetAppearance", "applyConditionalSettings", "pageBuilder.applyVisibleWhen", "applyVisibleRoles"} {
		for k := range src.reads(fn).keys {
			tail[strings.ToLower(k)] = true
		}
	}
	want := lowerSet(append(append(append([]string(nil), appearanceKeys...), visibilityKeys...), "Editable", "EditableIf"))
	if strings.Join(sortedNameSet(tail), ",") != strings.Join(sortedNameSet(want), ",") {
		t.Errorf("buildWidgetV3's tail reads %v, want %v (appearanceKeys + visibilityKeys + editability)",
			sortedNameSet(tail), sortedNameSet(want))
	}
}

// genWidgetConstructors build the gen element the writer encodes for each
// stored $Type. Its setters are what the writer can store: applyWidgetBase
// writes visibility only through SetConditionalVisibilitySettings, and drops
// it silently on a type without one.
var genWidgetConstructors = map[string]func() element.Element{
	"Forms$DataView":            func() element.Element { return genPg.NewDataView() },
	"Forms$ListView":            func() element.Element { return genPg.NewListView() },
	"Forms$TextBox":             func() element.Element { return genPg.NewTextBox() },
	"Forms$TextArea":            func() element.Element { return genPg.NewTextArea() },
	"Forms$DatePicker":          func() element.Element { return genPg.NewDatePicker() },
	"Forms$DropDown":            func() element.Element { return genPg.NewDropDown() },
	"Forms$CheckBox":            func() element.Element { return genPg.NewCheckBox() },
	"Forms$RadioButtonGroup":    func() element.Element { return genPg.NewRadioButtonGroup() },
	"Forms$FileManager":         func() element.Element { return genPg.NewFileManager() },
	"Forms$DynamicText":         func() element.Element { return genPg.NewDynamicText() },
	"Forms$Title":               func() element.Element { return genPg.NewTitle() },
	"Forms$Label":               func() element.Element { return genPg.NewLabel() },
	"Forms$ActionButton":        func() element.Element { return genPg.NewActionButton() },
	"Forms$SidebarToggleButton": func() element.Element { return genPg.NewSidebarToggleButton() },
	"Forms$DivContainer":        func() element.Element { return genPg.NewDivContainer() },
	"Forms$GroupBox":            func() element.Element { return genPg.NewGroupBox() },
	"Forms$TabControl":          func() element.Element { return genPg.NewTabContainer() },
	"Forms$LayoutGrid":          func() element.Element { return genPg.NewLayoutGrid() },
	"Forms$NavigationList":      func() element.Element { return genPg.NewNavigationList() },
	"Forms$SnippetCallWidget":   func() element.Element { return genPg.NewSnippetCallWidget() },
	"Forms$StaticImageViewer":   func() element.Element { return genPg.NewStaticImageViewer() },
	"Forms$ImageViewer":         func() element.Element { return genPg.NewDynamicImageViewer() },
	"Forms$ScrollContainer":     func() element.Element { return genPg.NewScrollContainer() },
	"Forms$Header":              func() element.Element { return genPg.NewHeader() },
	"Forms$Placeholder":         func() element.Element { return genPg.NewPlaceholder() },
	"Forms$NavigationTree":      func() element.Element { return genPg.NewNavigationTree() },
	"Forms$MenuBar":             func() element.Element { return genPg.NewMenuBar() },
	"Forms$SimpleMenuBar":       func() element.Element { return genPg.NewSimpleMenuBar() },
}

// A tail kind accepts Visible exactly when its stored type can hold it.
func TestNativeWidgetProps_TailMatchesGen(t *testing.T) {
	for kind, k := range nativeWidgetKinds {
		if !k.tail {
			continue
		}
		ctor, ok := genWidgetConstructors[k.stored]
		if !ok {
			t.Errorf("%s: no gen constructor for %s in genWidgetConstructors", kind, k.stored)
			continue
		}
		g := ctor()
		if g.TypeName() != k.stored {
			t.Errorf("%s: genWidgetConstructors[%s] builds %s", kind, k.stored, g.TypeName())
		}
		_, hasVisibility := g.(interface {
			SetConditionalVisibilitySettings(element.Element)
		})
		if hasVisibility != k.visible {
			t.Errorf("%s (%s): visible = %v, but the gen type the writer encodes has a "+
				"ConditionalVisibilitySettings setter: %v", kind, k.stored, k.visible, hasVisibility)
		}
		if _, ok := g.(interface{ SetAppearance(element.Element) }); !ok {
			t.Errorf("%s (%s): the tail writes appearance, but the gen type has no SetAppearance", kind, k.stored)
		}
	}
}

// describeCaseKinds maps each $Type case of outputWidgetMDLV3 to the kinds its
// body prints. A case printing several elements (a layout grid with its rows
// and columns) is held to the union of their sets.
var describeCaseKinds = map[string][]string{
	"Forms$ScrollContainer":       {"scrollcontainer"},
	"Forms$Header":                {"layoutheader", "layoutheader region"},
	"Forms$ScrollContainerRegion": {"scrollcontainer region"},
	"Forms$Placeholder":           {"placeholder"},
	"Forms$NavigationTree":        {"navigationtree", "menubar", "simplemenubar"},
	"Forms$TabControl":            {"tabcontainer"},
	"Forms$DivContainer":          {"container"},
	"Forms$GroupBox":              {"groupbox"},
	"Forms$LayoutGrid":            {"layoutgrid", "layoutgrid row", "layoutgrid column"},
	"Forms$DynamicText":           {"dynamictext"},
	"Forms$ActionButton":          {"actionbutton", "linkbutton", "sidebartoggle"},
	"Forms$Title":                 {"title"},
	"Forms$DataView":              {"dataview"},
	"Forms$TextBox":               {"textbox"},
	"Forms$TextArea":              {"textarea"},
	"Forms$FileManager":           {"filemanager"},
	"Forms$DatePicker":            {"datepicker"},
	"Forms$RadioButtons":          {"radiobuttons"},
	"Forms$CheckBox":              {"checkbox"},
	"Forms$NavigationList":        {"navigationlist", "navigationlist item"},
	"Forms$Label":                 {"label"},
	"Forms$StaticImageViewer":     {"staticimage"},
	"Forms$ImageViewer":           {"dynamicimage"},
	"Forms$SnippetCallWidget":     {"snippetcall"},
	"Forms$ListViewTemplate":      {"listview template"},
	"Forms$ListView":              {"listview"},
}

// describePrintsKeyRe matches the `Key: ` a describe format string starts a
// property with.
var describePrintsKeyRe = regexp.MustCompile(`^([A-Z][A-Za-z]+): `)

// describe → exec must not refuse its own output: every key a describe case
// prints is one its kind takes. Visibility and editability are printed only
// when the stored document has them, which the metamodel decides per type
// (TestNativeWidgetProps_TailMatchesGen), so they are not judged here.
func TestNativeWidgetProps_DescribeEmitsOnlyAccepted(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "cmd_pages_describe_output.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]*goast.FuncDecl{}
	for _, d := range f.Decls {
		if fd, ok := d.(*goast.FuncDecl); ok && fd.Body != nil && fd.Recv == nil {
			funcs[fd.Name.Name] = fd
		}
	}
	out := funcs["outputWidgetMDLV3"]
	if out == nil {
		t.Fatal("outputWidgetMDLV3 not found")
	}
	// printed collects the `Key: ` literals of a body and of the functions it
	// calls in this file (not outputWidgetMDLV3 itself: that prints children).
	var printed func(n goast.Node, keys map[string]bool, seen map[string]bool)
	printed = func(n goast.Node, keys map[string]bool, seen map[string]bool) {
		goast.Inspect(n, func(m goast.Node) bool {
			switch x := m.(type) {
			case *goast.BasicLit:
				if lit, ok := stringLit(x); ok {
					if sm := describePrintsKeyRe.FindStringSubmatch(lit); sm != nil {
						keys[sm[1]] = true
					}
				}
			case *goast.CallExpr:
				name := calleeName(x)
				// widgetConditionMDL("Visible", …) and actionProp("Action", …)
				// take the key as their first argument.
				if (name == "widgetConditionMDL" || name == "actionProp") && len(x.Args) > 0 {
					if lit, ok := stringLit(x.Args[0]); ok {
						keys[lit] = true
					}
				}
				if fd := funcs[name]; fd != nil && name != "outputWidgetMDLV3" && !seen[name] {
					seen[name] = true
					printed(fd.Body, keys, seen)
				}
			}
			return true
		})
	}
	judgedLater := lowerSet(append(append([]string(nil), visibilityKeys...), "Editable", "EditableIf"))
	covered := map[string]bool{}
	goast.Inspect(out.Body, func(n goast.Node) bool {
		cc, ok := n.(*goast.CaseClause)
		if !ok {
			return true
		}
		var kinds []string
		for _, e := range cc.List {
			if lit, ok := stringLit(e); ok {
				if ks, ok := describeCaseKinds[lit]; ok {
					kinds = ks
					covered[lit] = true
				}
			}
		}
		if kinds == nil {
			return true
		}
		keys := map[string]bool{}
		for _, stmt := range cc.Body {
			printed(stmt, keys, map[string]bool{})
		}
		for key := range keys {
			if judgedLater[strings.ToLower(key)] {
				continue
			}
			accepted := false
			for _, kind := range kinds {
				keyword := kind
				if i := strings.LastIndex(kind, " "); i >= 0 {
					keyword = kind[i+1:]
				}
				if nativeWidgetAccepts(kind, keyword, key) || nativeWidgetKeyOwnedElsewhere(kind, keyword, key) {
					accepted = true
				}
			}
			if !accepted {
				t.Errorf("describe prints `%s:` for %v, which none of them takes — describe → exec would refuse its own output", key, kinds)
			}
		}
		return false
	})
	for typ := range describeCaseKinds {
		if !covered[typ] {
			t.Errorf("describeCaseKinds names %s, but outputWidgetMDLV3 has no such case", typ)
		}
	}
}
