// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMsg "github.com/mendixlabs/mxcli/modelsdk/gen/messagedefinitions"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// MessageDefinition2 documents (ako/mxcli#987).
//
// Mendix 11.15 replaced the MessageDefinitionCollection with one
// MessageDefinitions$MessageDefinition2 document per definition. Measured with
// `mx convert` 11.15.0 on an 11.14 project: the collection unit becomes a
// Projects$Folder of the same name and unit ID, and each entry becomes a
// document inside it with Name, Documentation, Excluded, ExportLevel and
// ExposedEntity. The ExposedEntity tree is byte-identical to the old entry's
// apart from $IDs, so the element conversion is the collection's, unchanged.

const messageDefinitionDocumentType = "MessageDefinitions$MessageDefinition2"

// ListMessageDefinitionDocuments reads every MessageDefinition2 unit.
func (b *Backend) ListMessageDefinitionDocuments() ([]*model.MessageDefinitionDocument, error) {
	units, err := mprread.ListUnitsWithContainer[*genMsg.MessageDefinition2](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*model.MessageDefinitionDocument, 0, len(units))
	for _, u := range units {
		out = append(out, messageDocumentFromGen(u.Element, string(u.ContainerID)))
	}
	return out, nil
}

// messageDocumentFromGen converts one stored document to the semantic model.
func messageDocumentFromGen(g *genMsg.MessageDefinition2, containerID string) *model.MessageDefinitionDocument {
	d := &model.MessageDefinitionDocument{
		ContainerID:   model.ID(containerID),
		Name:          g.Name(),
		Documentation: g.Documentation(),
		Excluded:      g.Excluded(),
		ExportLevel:   g.ExportLevel(),
		Root:          exposedNodeFromGen(g.ExposedEntity()),
	}
	d.ID = model.ID(g.ID())
	d.TypeName = messageDefinitionDocumentType
	return d
}

// CreateMessageDefinitionDocument writes a new MessageDefinition2 document.
func (b *Backend) CreateMessageDefinitionDocument(d *model.MessageDefinitionDocument) error {
	if d == nil {
		return fmt.Errorf("CreateMessageDefinitionDocument: nil document")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateMessageDefinitionDocument: not connected for writing")
	}
	if d.ID == "" {
		d.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeMessageDefinitionDocument(d)
	if err != nil {
		return err
	}
	return b.writer.InsertUnit(string(d.ID), string(d.ContainerID), "Documents",
		messageDefinitionDocumentType, contents)
}

// UpdateMessageDefinitionDocument rewrites a document in place, keeping its ID.
func (b *Backend) UpdateMessageDefinitionDocument(d *model.MessageDefinitionDocument) error {
	if d == nil {
		return fmt.Errorf("UpdateMessageDefinitionDocument: nil document")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateMessageDefinitionDocument: not connected for writing")
	}
	contents, err := encodeMessageDefinitionDocument(d)
	if err != nil {
		return err
	}
	return b.writer.UpdateRawUnit(string(d.ID), contents)
}

// DeleteMessageDefinitionDocument removes a document by ID.
func (b *Backend) DeleteMessageDefinitionDocument(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteMessageDefinitionDocument: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}

func encodeMessageDefinitionDocument(d *model.MessageDefinitionDocument) ([]byte, error) {
	g, err := messageDocumentToGen(d)
	if err != nil {
		return nil, err
	}
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return nil, fmt.Errorf("message definition %s: encode: %w", d.Name, err)
	}
	return contents, nil
}

func messageDocumentToGen(d *model.MessageDefinitionDocument) (*genMsg.MessageDefinition2, error) {
	g := genMsg.NewMessageDefinition2()
	g.SetID(element.ID(d.ID))
	g.SetName(d.Name)
	g.SetDocumentation(d.Documentation)
	g.SetExcluded(d.Excluded)
	exportLevel := d.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden" // what `mx convert` writes
	}
	g.SetExportLevel(exportLevel)
	root, err := messageNodeToGen(d.Root, "")
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("message definition %s has no root element", d.Name)
	}
	g.SetExposedEntity(root)
	return g, nil
}
