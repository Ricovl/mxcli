// SPDX-License-Identifier: Apache-2.0

package mpk

import (
	"reflect"
	"testing"
)

// An association property's <associationTypes> and selectableObjects are part of
// the widget DEFINITION (ValueType.AssociationTypes / SelectableObjectsProperty).
// Not reading them left augmentation nothing to write, so a Data Widgets 2.30.3
// column's filterAssociation was emitted without them — CE0463 on a 10.24.0
// project (mendixlabs/mxcli#1424).
func TestParse_ReadsAssociationTypesAndSelectableObjects(t *testing.T) {
	const widgetXML = `<?xml version="1.0" encoding="utf-8"?>
<widget id="com.example.W" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>W</name>
  <properties>
    <propertyGroup caption="General">
      <property key="columns" type="object" isList="true">
        <caption>Columns</caption><description/>
        <properties>
          <propertyGroup caption="Column">
            <property key="filterAssociation" type="association" selectableObjects="filterAssociationOptions" required="false" dataSource="../datasource">
              <caption>Entity</caption><description/>
              <associationTypes>
                <associationType name="Reference" />
                <associationType name="ReferenceSet" />
              </associationTypes>
            </property>
          </propertyGroup>
        </properties>
      </property>
      <property key="topAssoc" type="association" selectableObjects="opts">
        <caption>Top</caption><description/>
        <associationTypes><associationType name="Reference" /></associationTypes>
      </property>
      <property key="plain" type="attribute">
        <caption>Plain</caption><description/>
        <attributeTypes><attributeType name="String" /></attributeTypes>
      </property>
    </propertyGroup>
  </properties>
</widget>`

	def := parseWidgetXML(t, widgetXML)
	byKey := map[string]PropertyDef{}
	for _, p := range def.Properties {
		byKey[p.Key] = p
	}

	nested := byKey["columns"].Children[0]
	if !reflect.DeepEqual(nested.AssociationTypes, []string{"Reference", "ReferenceSet"}) {
		t.Errorf("nested AssociationTypes = %v", nested.AssociationTypes)
	}
	if nested.SelectableObjects != "filterAssociationOptions" {
		t.Errorf("nested SelectableObjects = %q", nested.SelectableObjects)
	}
	if top := byKey["topAssoc"]; !reflect.DeepEqual(top.AssociationTypes, []string{"Reference"}) || top.SelectableObjects != "opts" {
		t.Errorf("top-level association = %v / %q", top.AssociationTypes, top.SelectableObjects)
	}
	if p := byKey["plain"]; len(p.AssociationTypes) != 0 || p.SelectableObjects != "" {
		t.Errorf("plain attribute = %v / %q, want none", p.AssociationTypes, p.SelectableObjects)
	}
}
