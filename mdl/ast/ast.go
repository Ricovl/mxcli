// SPDX-License-Identifier: Apache-2.0

// Package ast defines the Abstract Syntax Tree nodes for MDL (Mendix Definition Language).
// This package contains types for the domain model subset: entities, attributes,
// associations, enumerations, and view entities.
package ast

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/langver"
)

// Statement represents any MDL statement that can be executed.
type Statement interface {
	isStatement()
}

// Position represents a location in the domain model canvas.
type Position struct {
	X int
	Y int
}

// QualifiedName represents a module-qualified name like "Module.Entity".
type QualifiedName struct {
	Module string
	Name   string
}

func (q QualifiedName) String() string {
	if q.Module == "" {
		return q.Name
	}
	return q.Module + "." + q.Name
}

// ============================================================================
// Program
// ============================================================================

// Program represents a complete MDL program (sequence of statements).
type Program struct {
	Statements []Statement
	// DocumentAnnotations records every annotation written before a CREATE —
	// `@excluded`, `@position`, `@applyentityaccess` and anything misspelled —
	// paired with the kind of document it was written on.
	//
	// The grammar lets ANY create statement carry annotations while only seven
	// document kinds read one, so an annotation on the wrong document, or with a
	// typo in it, parsed and did nothing. That is the failure MDL059 already
	// refuses one node family over: whatever the annotation was meant to express
	// is lost in silence.
	//
	// Every annotation is recorded rather than only the unrecognised ones, so
	// which names a document accepts stays a single decision in the validator
	// next to knownActivityAnnotations, instead of being spread across the seven
	// visitor sites that read them.
	DocumentAnnotations []DocumentAnnotation
	// Deprecations records every use of a deprecated spelling registered in
	// mdl/deprecation, in source order. Both spellings build the same
	// statements, so this is the only trace of which one the source used; it
	// drives the MDL-DEPRnnn warnings and nothing else may branch on it.
	Deprecations []DeprecatedSpelling

	// LanguageVersion is the MDL language version the script is written in:
	// the number in its `mdl <n>;` header, or mdl 0 when it has none
	// (ADR-0011). A construct whose meaning differs between versions reads it
	// through langver.Change; nothing may assume the latest.
	LanguageVersion langver.Version
	// LanguageHeaderLine is the 1-based line of the header, 0 when the script
	// has none.
	LanguageHeaderLine int
	// LanguageNotes are the constructs kept at their older meaning because of
	// LanguageVersion, one per occurrence, for check and exec to warn on.
	LanguageNotes []LanguageNote
}

// DeprecatedSpelling is one use of a deprecated spelling in the source.
type DeprecatedSpelling struct {
	// Code is the registry code, MDL-DEPRnnn.
	Code string
	// Line and Column locate the deprecated token (1-based line, 0-based
	// column, as ANTLR reports them).
	Line   int
	Column int
	// Subject says what the spelling was used on, in MDL's own words ("entity",
	// "microflow", …); empty when there is nothing more specific to say.
	Subject string
	// Fix is the structural rewrite of this use to the canonical form, for an
	// entry whose rewrite is not a keyword swap (deprecation.Rewrite.Structural).
	// Nil when this use has none; NoFix then says why.
	Fix   *Fix
	NoFix string
}

// LanguageNote is one construct whose meaning depends on the language version,
// kept at the meaning of the version the script is written in.
type LanguageNote struct {
	Line    int    // 1-based source line of the construct
	Code    string // the langver.Change's rule ID
	Message string
	// Fix rewrites the construct so that under the new version it still means
	// what it means here — what `fmt --upgrade --header` applies before adding
	// the header. Nil when there is no mechanical rewrite; NoFix then says why.
	// A Fix with no edits is a construct already spelled so that it keeps its
	// meaning under the new version.
	Fix   *Fix
	NoFix string
}

// Fix is a mechanical source rewrite: edits in the coordinates the parser read
// the script in, computed from the parse tree by the visitor that recorded the
// construct. Edits may not overlap.
type Fix struct {
	Edits []TextEdit
}

// TextEdit replaces the runes [Start, Stop) of the script with Text. Offsets
// count runes (code points) from the start of the script, as ANTLR's character
// stream does; Start == Stop is an insertion.
type TextEdit struct {
	Start, Stop int
	Text        string
}

// DocumentAnnotation is one annotation written before a CREATE statement.
type DocumentAnnotation struct {
	// Kind is the document it was written on, in MDL's own words ("microflow",
	// "nanoflow", "entity", …), so the message can name it.
	Kind string
	// Name is the annotation, lower-cased and without the "@".
	Name string
	// Target is the document's qualified name where the visitor could read one,
	// for a message that points at the right statement in a long script.
	Target string
}

// ============================================================================
// Move Statement
// ============================================================================

// DocumentType represents the type of document being moved.
type DocumentType string

// The doctypes MOVE accepts. The value is the MDL spelling, which is also what
// the executor prints back — so an unrecognised one cannot be mistaken for a
// recognised one in output.
//
// Only ENTITY is not a top-level document unit: it lives inside a domain model
// and its move converts associations rather than reparenting a row. Everything
// else here reduces to one containment update, which is why doctypes can be
// added to this list without a handler each.
const (
	DocumentTypePage                 DocumentType = "PAGE"
	DocumentTypeMicroflow            DocumentType = "MICROFLOW"
	DocumentTypeSnippet              DocumentType = "SNIPPET"
	DocumentTypeNanoflow             DocumentType = "NANOFLOW"
	DocumentTypeRule                 DocumentType = "RULE"
	DocumentTypeEntity               DocumentType = "ENTITY"
	DocumentTypeEnumeration          DocumentType = "ENUMERATION"
	DocumentTypeConstant             DocumentType = "CONSTANT"
	DocumentTypeDatabaseConnection   DocumentType = "DATABASE CONNECTION"
	DocumentTypeJavaAction           DocumentType = "JAVA ACTION"
	DocumentTypeODataService         DocumentType = "ODATA SERVICE"
	DocumentTypeBuildingBlock        DocumentType = "BUILDING BLOCK"
	DocumentTypeLayout               DocumentType = "LAYOUT"
	DocumentTypeMenu                 DocumentType = "MENU"
	DocumentTypeWorkflow             DocumentType = "WORKFLOW"
	DocumentTypeQueue                DocumentType = "QUEUE"
	DocumentTypeScheduledEvent       DocumentType = "SCHEDULED EVENT"
	DocumentTypeRegularExpression    DocumentType = "REGULAR EXPRESSION"
	DocumentTypeJsonStructure        DocumentType = "JSON STRUCTURE"
	DocumentTypeImportMapping        DocumentType = "IMPORT MAPPING"
	DocumentTypeExportMapping        DocumentType = "EXPORT MAPPING"
	DocumentTypeJavaScriptAction     DocumentType = "JAVASCRIPT ACTION"
	DocumentTypeDataTransformer      DocumentType = "DATA TRANSFORMER"
	DocumentTypeImageCollection      DocumentType = "IMAGE COLLECTION"
	DocumentTypeIconCollection       DocumentType = "ICON COLLECTION"
	DocumentTypeRestClient           DocumentType = "REST CLIENT"
	DocumentTypePublishedRestService DocumentType = "PUBLISHED REST SERVICE"
	DocumentTypeODataClient          DocumentType = "ODATA CLIENT"
	DocumentTypeBusinessEventService DocumentType = "BUSINESS EVENT SERVICE"
	DocumentTypeModel                DocumentType = "MODEL"
	DocumentTypeAgent                DocumentType = "AGENT"
	DocumentTypeKnowledgeBase        DocumentType = "KNOWLEDGE BASE"
	DocumentTypeConsumedMCPService   DocumentType = "CONSUMED MCP SERVICE"
)

// MoveDocumentTypeByKeyword maps a moveDocumentType grammar rule's text to its
// DocumentType. The rule's GetText() concatenates its tokens with no separator,
// so `IMPORT MAPPING` arrives as "IMPORTMAPPING".
//
// The single registry for the doctypes MOVE accepts: the visitor reads it to
// build the statement, and the executor reads its values to decide whether a
// document it found is the kind the statement named. Keeping one list is the
// point — a second copy is how the MOVE FOLDER discriminator went stale before.
var MoveDocumentTypeByKeyword = map[string]DocumentType{
	"PAGE":                 DocumentTypePage,
	"MICROFLOW":            DocumentTypeMicroflow,
	"NANOFLOW":             DocumentTypeNanoflow,
	"RULE":                 DocumentTypeRule,
	"SNIPPET":              DocumentTypeSnippet,
	"BUILDINGBLOCK":        DocumentTypeBuildingBlock,
	"LAYOUT":               DocumentTypeLayout,
	"MENU":                 DocumentTypeMenu,
	"ENUMERATION":          DocumentTypeEnumeration,
	"CONSTANT":             DocumentTypeConstant,
	"WORKFLOW":             DocumentTypeWorkflow,
	"QUEUE":                DocumentTypeQueue,
	"SCHEDULEDEVENT":       DocumentTypeScheduledEvent,
	"REGULAREXPRESSION":    DocumentTypeRegularExpression,
	"JSONSTRUCTURE":        DocumentTypeJsonStructure,
	"IMPORTMAPPING":        DocumentTypeImportMapping,
	"EXPORTMAPPING":        DocumentTypeExportMapping,
	"JAVAACTION":           DocumentTypeJavaAction,
	"JAVASCRIPTACTION":     DocumentTypeJavaScriptAction,
	"DATABASECONNECTION":   DocumentTypeDatabaseConnection,
	"DATATRANSFORMER":      DocumentTypeDataTransformer,
	"IMAGECOLLECTION":      DocumentTypeImageCollection,
	"ICONCOLLECTION":       DocumentTypeIconCollection,
	"RESTCLIENT":           DocumentTypeRestClient,
	"PUBLISHEDRESTSERVICE": DocumentTypePublishedRestService,
	"ODATACLIENT":          DocumentTypeODataClient,
	"ODATASERVICE":         DocumentTypeODataService,
	"BUSINESSEVENTSERVICE": DocumentTypeBusinessEventService,
	"MODEL":                DocumentTypeModel,
	"AIMODEL":              DocumentTypeModel,
	"AGENT":                DocumentTypeAgent,
	"KNOWLEDGEBASE":        DocumentTypeKnowledgeBase,
	"CONSUMEDMCPSERVICE":   DocumentTypeConsumedMCPService,
	// The Studio Pro names (R10); the old spellings above stay as aliases.
	"TASKQUEUE":             DocumentTypeQueue,
	"CONSUMEDRESTSERVICE":   DocumentTypeRestClient,
	"CONSUMEDODATASERVICE":  DocumentTypeODataClient,
	"PUBLISHEDODATASERVICE": DocumentTypeODataService,
}

// IsMoveDocumentType reports whether spelling (lower-cased, spaced, e.g.
// "json structure") names a doctype MOVE accepts.
// Both the canonical spelling and a deprecated alias count ("task queue" and
// "queue").
func IsMoveDocumentType(spelling string) bool {
	_, ok := MoveDocumentTypeByKeyword[strings.ToUpper(strings.ReplaceAll(spelling, " ", ""))]
	return ok
}

// documentTypeStudioProNames spells the document types R10 renamed
// (ako/mxcli#755) as Studio Pro names them. The DocumentType values keep the
// old words because they are internal keys; this is what a user is shown.
var documentTypeStudioProNames = map[DocumentType]string{
	DocumentTypeQueue:        "task queue",
	DocumentTypeRestClient:   "consumed rest service",
	DocumentTypeODataClient:  "consumed odata service",
	DocumentTypeODataService: "published odata service",
}

// CanonicalSpelling is the lower-case canonical MDL spelling of a document
// type, for messages and advice ("consumed rest service", "json structure").
func (d DocumentType) CanonicalSpelling() string {
	if name, ok := documentTypeStudioProNames[d]; ok {
		return name
	}
	return strings.ToLower(string(d))
}

// MoveStmt represents: MOVE PAGE/MICROFLOW/SNIPPET/NANOFLOW/ENTITY/ENUMERATION Module.Name TO FOLDER 'path' IN Module
type MoveStmt struct {
	DocumentType DocumentType  // PAGE, MICROFLOW, SNIPPET, NANOFLOW, ENTITY, ENUMERATION
	Name         QualifiedName // Source document qualified name
	Folder       string        // Target folder path (empty = module root)
	TargetModule string        // Target module name (empty = same module)
}

func (s *MoveStmt) isStatement() {}
