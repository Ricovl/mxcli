// SPDX-License-Identifier: Apache-2.0

// Package executor - Image collection commands (CREATE/DROP IMAGE COLLECTION)
package executor

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// execCreateImageCollection handles CREATE IMAGE COLLECTION statements.
func execCreateImageCollection(ctx *ExecContext, s *ast.CreateImageCollectionStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Check if image collection already exists
	existing := findImageCollection(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("image collection", s.Name.Module+"."+s.Name.Name)
	}

	var existingContainer model.ID
	if existing != nil {
		existingContainer = existing.ContainerID
	}
	containerID, err := containerForDocument(ctx, module.ID, s.Folder, existingContainer)
	if err != nil {
		return err
	}

	// Build ImageCollection
	ic := &types.ImageCollection{
		ContainerID:   containerID,
		Name:          s.Name.Name,
		ExportLevel:   s.ExportLevel,
		Documentation: s.Comment,
	}
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if existing != nil {
		ic.Documentation = carriedDocumentation(s.DocumentationSet, s.Comment, existing.Documentation)
	}
	if existing != nil {
		ic.ID = existing.ID
		// Excluded is model state, not script state (#914).
		ic.Excluded = existing.Excluded
	}

	// Load the images: inline Data (what describe writes), or a file, which a
	// relative path names relative to the script, as `execute script` does.
	for _, item := range s.Images {
		data, format := item.Data, item.Format
		if !item.HasData {
			filePath, err := ctx.ResolveScriptRelative(item.FilePath)
			if err != nil {
				return err
			}
			data, err = os.ReadFile(filePath)
			if err != nil {
				return mdlerrors.NewBackend(fmt.Sprintf("read image file %q", item.FilePath), err)
			}
			if format == "" {
				format = extToImageFormat(filepath.Ext(filePath))
			}
		}
		if format == "" {
			format = sniffImageFormat(data)
		}
		ic.Images = append(ic.Images, types.Image{
			Name:   item.Name,
			Data:   data,
			Format: format,
		})
	}

	if existing != nil {
		if err := ctx.Backend.UpdateImageCollection(ic); err != nil {
			return mdlerrors.NewBackend("update image collection", err)
		}
		if _, err := applyDocumentFolder(ctx, ic.ID, existingContainer, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "image collection: %s", s.Name)
	} else {
		if err := ctx.Backend.CreateImageCollection(ic); err != nil {
			return mdlerrors.NewBackend("create image collection", err)
		}
		fmt.Fprintf(ctx.Output, "Created image collection: %s\n", s.Name)
	}

	// Invalidate hierarchy cache so the collection's container is visible
	invalidateHierarchy(ctx)
	return nil
}

// execDropImageCollection handles DROP IMAGE COLLECTION statements.
func execDropImageCollection(ctx *ExecContext, s *ast.DropImageCollectionStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	ic := findImageCollection(ctx, s.Name.Module, s.Name.Name)
	if ic == nil {
		return mdlerrors.NewNotFound("image collection", s.Name.String())
	}

	if err := ctx.Backend.DeleteImageCollection(string(ic.ID)); err != nil {
		return mdlerrors.NewBackend("delete image collection", err)
	}

	fmt.Fprintf(ctx.Output, "Dropped image collection: %s\n", s.Name)
	return nil
}

// describeImageCollection handles DESCRIBE IMAGE COLLECTION Module.Name.
func describeImageCollection(ctx *ExecContext, name ast.QualifiedName) error {
	ic := findImageCollection(ctx, name.Module, name.Name)
	if ic == nil {
		return mdlerrors.NewNotFound("image collection", name.String())
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(ic.ContainerID)
	modName := h.GetModuleName(modID)

	if ic.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", ic.Documentation)
	}

	exportLevel := ic.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}

	qualifiedName := fmt.Sprintf("%s.%s", modName, ic.Name)

	if len(ic.Images) == 0 {
		fmt.Fprintf(ctx.Output, "create or modify image collection %s%s", qualifiedName, describeFolderClause(ctx, ic.ContainerID))
		if exportLevel != "Hidden" {
			fmt.Fprintf(ctx.Output, " export level '%s'", exportLevel)
		}
		fmt.Fprintln(ctx.Output, ";")
		return nil
	}

	fmt.Fprintf(ctx.Output, "create or modify image collection %s%s", qualifiedName, describeFolderClause(ctx, ic.ContainerID))
	if exportLevel != "Hidden" {
		fmt.Fprintf(ctx.Output, " export level '%s'", exportLevel)
	}
	fmt.Fprintln(ctx.Output, " {")

	// The images are written into the statement, base64-encoded. They used to
	// be written to /tmp/mxcli-preview and named by that path, so the output
	// only replayed on the machine that described it, until /tmp was cleared,
	// and a diff of it showed a path rather than a changed image
	// (ako/mxcli#707). Format is written only where the bytes do not say it.
	for _, img := range ic.Images {
		props := "Data: '" + base64.StdEncoding.EncodeToString(img.Data) + "'"
		if f := img.Format; f != "" && f != sniffImageFormat(img.Data) {
			props += ", Format: " + strings.ToLower(f)
		}
		fmt.Fprintf(ctx.Output, "  image %s ( %s )\n", mdlIdent(img.Name), props)
	}

	fmt.Fprintln(ctx.Output, "};")
	return nil
}

// extToImageFormat converts a file extension to a Mendix ImageFormat value.
func extToImageFormat(ext string) string {
	switch strings.ToLower(ext) {
	case ".svg":
		return "Svg"
	case ".gif":
		return "Gif"
	case ".jpg", ".jpeg":
		return "Jpg"
	case ".bmp":
		return "Bmp"
	case ".webp":
		return "Webp"
	default:
		return "Png"
	}
}

// listImageCollections handles SHOW IMAGE COLLECTION [IN module].
func listImageCollections(ctx *ExecContext, moduleName string) error {
	collections, err := ctx.Backend.ListImageCollections()
	if err != nil {
		return mdlerrors.NewBackend("list image collections", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	result := &TableResult{
		Columns: []string{"Image Collection", "Export Level", "Images"},
	}

	for _, ic := range collections {
		modID := h.FindModuleID(ic.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}

		qualifiedName := fmt.Sprintf("%s.%s", modName, ic.Name)
		exportLevel := ic.ExportLevel
		if exportLevel == "" {
			exportLevel = "Hidden"
		}
		result.Rows = append(result.Rows, []any{qualifiedName, exportLevel, len(ic.Images)})
	}

	result.Summary = fmt.Sprintf("(%d image collection(s))", len(result.Rows))
	return writeResult(ctx, result)
}

// findImageCollection finds an image collection by module and name.
func findImageCollection(ctx *ExecContext, moduleName, collectionName string) *types.ImageCollection {
	collections, err := ctx.Backend.ListImageCollections()
	if err != nil {
		return nil
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}

	for _, ic := range collections {
		modID := h.FindModuleID(ic.ContainerID)
		modName := h.GetModuleName(modID)
		if ic.Name == collectionName && modName == moduleName {
			return ic
		}
	}
	return nil
}

// sniffImageFormat is the Mendix ImageFormat the bytes of an image declare
// themselves: a PNG, JPEG, GIF, BMP or WebP signature, or an SVG document. ""
// when they declare none.
func sniffImageFormat(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "Png"
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "Jpg"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "Gif"
	case bytes.HasPrefix(data, []byte("BM")):
		return "Bmp"
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "Webp"
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	if bytes.Contains(bytes.ToLower(head), []byte("<svg")) {
		return "Svg"
	}
	return ""
}
