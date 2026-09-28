// SPDX-License-Identifier: Apache-2.0

package ast

// ImageItem represents an image to add to a collection: IMAGE "name" FROM FILE 'path'.
type ImageItem struct {
	Name     string // Image name (e.g. "logo")
	FilePath string // Path to image file on disk; empty when Data is set
	// Data is the image itself, from `Data: '<base64>'`: what describe writes,
	// so its output does not depend on a file it wrote somewhere (ako/mxcli#707).
	Data    []byte
	HasData bool
	// Format is the Mendix ImageFormat from `Format: <fmt>` (Png, Jpg, Gif,
	// Svg, Bmp, Webp). Empty: taken from the file extension, or from the bytes.
	Format string
}

// CreateImageCollectionStmt represents:
//
//	CREATE IMAGE COLLECTION Module.Name [EXPORT LEVEL 'Public'] [COMMENT '...'] [(IMAGE "name" FROM FILE 'path', ...)]
type CreateImageCollectionStmt struct {
	CreateGuard             // `create … if not exists` (ako/mxcli#731)
	Folder           string // Folder path within module (empty = leave placement alone)
	Name             QualifiedName
	CreateOrModify   bool
	ExportLevel      string // "Hidden" (default) or "Public"
	Comment          string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
	Images           []ImageItem
}

func (s *CreateImageCollectionStmt) isStatement() {}

// DropImageCollectionStmt represents: DROP IMAGE COLLECTION Module.Name
type DropImageCollectionStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropImageCollectionStmt) isStatement() {}
