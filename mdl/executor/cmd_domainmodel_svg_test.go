// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

var regexpLines = regexp.MustCompile(`<g class="association"><title>[^<]*</title>(<line [^>]*>)`)

func svgTestModel() ([]*domainmodel.Entity, []*domainmodel.Association) {
	customer := &domainmodel.Entity{Name: "Customer", Persistable: true, Location: model.Point{X: 100, Y: 100}}
	customer.ID = "c"
	customer.Attributes = []*domainmodel.Attribute{{Name: "Name", Type: &domainmodel.StringAttributeType{Length: 200}}}
	order := &domainmodel.Entity{Name: "Order", Persistable: true, Location: model.Point{X: 400, Y: 120}}
	order.ID = "o"
	filter := &domainmodel.Entity{Name: "OrderFilter", Persistable: false, Location: model.Point{X: 100, Y: 300}}
	filter.ID = "f"
	account := &domainmodel.Entity{Name: "Account", Persistable: true, Location: model.Point{X: 400, Y: 300}, GeneralizationRef: "System.User"}
	account.ID = "a"
	// Order_Customer: from Order (owns the reference) to Customer.
	assoc := &domainmodel.Association{Name: "Order_Customer", ParentID: "o", ChildID: "c", Type: domainmodel.AssociationTypeReference}
	tags := &domainmodel.Association{Name: "Order_Tag", ParentID: "o", ChildID: "elsewhere", Type: domainmodel.AssociationTypeReferenceSet}
	return []*domainmodel.Entity{customer, order, filter, account}, []*domainmodel.Association{assoc, tags}
}

// The SVG has to be well-formed: the mod hands it to the surface as is, and a
// broken document draws nothing.
func TestDomainModelSVG_IsWellFormed(t *testing.T) {
	entities, assocs := svgTestModel()
	svg := renderDomainModelSVG("Sales", entities, assocs)
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		_, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("not well-formed XML: %v\n%s", err, svg)
		}
	}
	if !strings.HasPrefix(svg, "<svg ") || !strings.Contains(svg, `viewBox="`) {
		t.Errorf("want an <svg> root with a viewBox, got %.80s", svg)
	}
}

// Entities sit where Studio Pro put them, with their attributes, and the box
// colour says what kind of entity it is.
func TestDomainModelSVG_DrawsEntitiesAtTheirStoredPositions(t *testing.T) {
	entities, assocs := svgTestModel()
	svg := renderDomainModelSVG("Sales", entities, assocs)
	for _, want := range []string{
		`<g class="entity persistent" transform="translate(100 100)">`,
		`<g class="entity persistent" transform="translate(400 120)">`,
		`<g class="entity nonpersistent" transform="translate(100 300)">`,
		">Customer<", ">Name: String<", ">OrderFilter<",
		">extends System.User<",
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %q in\n%s", want, svg)
		}
	}
}

// An association between two drawn entities is a line with its multiplicities;
// one to an entity of another module has nowhere to go and is left out rather
// than drawn to the origin.
func TestDomainModelSVG_DrawsAssociationsBetweenDrawnEntities(t *testing.T) {
	entities, assocs := svgTestModel()
	svg := renderDomainModelSVG("Sales", entities, assocs)
	if strings.Count(svg, `class="association"`) != 1 {
		t.Errorf("want exactly one association drawn (Order_Customer), got\n%s", svg)
	}
	if !strings.Contains(svg, "Order_Customer") {
		t.Error("the association's name is not in the drawing")
	}
	if strings.Contains(svg, "Order_Tag") {
		t.Error("an association to an entity outside the module was drawn")
	}
}

func TestDomainModelSVG_EscapesNames(t *testing.T) {
	e := &domainmodel.Entity{Name: "A<B>&C", Persistable: true}
	e.ID = "x"
	svg := renderDomainModelSVG("M", []*domainmodel.Entity{e}, nil)
	if strings.Contains(svg, "A<B>") || !strings.Contains(svg, "A&lt;B&gt;&amp;C") {
		t.Errorf("names are not escaped:\n%s", svg)
	}
}

func TestDomainModelSVG_EmptyModuleSaysSo(t *testing.T) {
	svg := renderDomainModelSVG("Empty", nil, nil)
	if !strings.Contains(svg, "no entities") {
		t.Errorf("an empty domain model should say so, got\n%s", svg)
	}
}

// Two associations between the same pair of entities are drawn side by side,
// not on top of each other (MicroflowSelection_Microflow and _Active in a real
// model printed their labels over one another).
func TestDomainModelSVG_SeparatesParallelAssociations(t *testing.T) {
	entities, _ := svgTestModel()
	assocs := []*domainmodel.Association{
		{Name: "Order_Customer", ParentID: "o", ChildID: "c", Type: domainmodel.AssociationTypeReference},
		{Name: "Order_Customer_Billing", ParentID: "o", ChildID: "c", Type: domainmodel.AssociationTypeReference},
	}
	svg := renderDomainModelSVG("Sales", entities, assocs)
	m := regexpLines.FindAllStringSubmatch(svg, -1)
	if len(m) < 2 {
		t.Fatalf("want two association lines, got %d", len(m))
	}
	if m[0][1] == m[1][1] {
		t.Errorf("parallel associations drawn on the same line: %s", m[0][1])
	}
}

var regexpEntity = regexp.MustCompile(`<g class="entity [a-z]+" transform="translate\((-?\d+) (-?\d+)\)"><title>[^<]*</title><rect width="(\d+)" height="(\d+)"`)

// Studio Pro places entities just far enough apart for its own box widths; a
// box estimated wider must not run into its neighbour (seen on a real
// Teamcenter model: names like TeamcenterConfigurationHelper overlapped the
// next entity). Boxes are clamped to the gap, the text clipped.
func TestDomainModelSVG_BoxesDoNotOverlapNeighbours(t *testing.T) {
	long := &domainmodel.Entity{Name: "TeamcenterConfigurationHelperWithAVeryLongName", Persistable: true, Location: model.Point{X: 0, Y: 0}}
	long.ID = "l"
	long.Attributes = []*domainmodel.Attribute{{Name: "EncryptedPasswordForTheConfiguration", Type: &domainmodel.StringAttributeType{}}}
	// Too close for even the largest spread to make room: the clamp must hold.
	right := &domainmodel.Entity{Name: "Next", Persistable: true, Location: model.Point{X: 60, Y: 10}}
	right.ID = "r"
	below := &domainmodel.Entity{Name: "Below", Persistable: true, Location: model.Point{X: 0, Y: 15}}
	below.ID = "b"
	svg := renderDomainModelSVG("M", []*domainmodel.Entity{long, right, below}, nil)

	type box struct{ x, y, w, h int }
	var boxes []box
	for _, m := range regexpEntity.FindAllStringSubmatch(svg, -1) {
		var b box
		fmt.Sscanf(m[1]+" "+m[2]+" "+m[3]+" "+m[4], "%d %d %d %d", &b.x, &b.y, &b.w, &b.h)
		boxes = append(boxes, b)
	}
	if len(boxes) != 3 {
		t.Fatalf("want 3 entity boxes, got %d:\n%s", len(boxes), svg)
	}
	for i, a := range boxes {
		for j, b := range boxes {
			if i < j && a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h {
				t.Errorf("boxes %d %+v and %d %+v overlap", i, a, j, b)
			}
		}
	}
	if !strings.Contains(svg, "<clipPath") {
		t.Error("a clamped box's text is not clipped")
	}
	if !strings.Contains(svg, "TeamcenterConfigurationHelperWithAVeryLongName") {
		t.Error("the full name should stay in the tooltip")
	}
}

// Studio Pro draws entities at a fixed width and cuts long names off, a
// common complaint. The diagram keeps Studio Pro's arrangement but spreads it
// apart (up to a cap) so each box is as wide as its longest line: nothing is
// clipped when spreading can make room.
func TestDomainModelSVG_SpreadsTheLayoutSoNamesFit(t *testing.T) {
	long := &domainmodel.Entity{Name: "TeamcenterConfigurationHelper", Persistable: true, Location: model.Point{X: 0, Y: 0}}
	long.ID = "l"
	long.Attributes = []*domainmodel.Attribute{{Name: "EncryptedPassword", Type: &domainmodel.StringAttributeType{}}}
	right := &domainmodel.Entity{Name: "Next", Persistable: true, Location: model.Point{X: 150, Y: 0}}
	right.ID = "r"
	svg := renderDomainModelSVG("M", []*domainmodel.Entity{long, right}, nil)
	if strings.Contains(svg, "<clipPath") {
		t.Errorf("text was clipped although the layout could spread to fit it:\n%s", svg)
	}
	m := regexpEntity.FindAllStringSubmatch(svg, -1)
	if len(m) != 2 || m[0][1] != "0" || m[1][1] == "150" {
		t.Errorf("want the left entity kept at 0 and the right one moved out from 150, got %v", m)
	}
}
