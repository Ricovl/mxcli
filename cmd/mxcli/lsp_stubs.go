// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"go.lsp.dev/protocol"
)

// Stub implementations for unimplemented Server interface methods.

func (s *mdlServer) WorkDoneProgressCancel(ctx context.Context, params *protocol.WorkDoneProgressCancelParams) error {
	return nil
}
func (s *mdlServer) LogTrace(ctx context.Context, params *protocol.LogTraceParams) error {
	return nil
}
func (s *mdlServer) SetTrace(ctx context.Context, params *protocol.SetTraceParams) error {
	return nil
}
func (s *mdlServer) CodeLens(ctx context.Context, params *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
	return nil, nil
}
func (s *mdlServer) CodeLensResolve(ctx context.Context, params *protocol.CodeLens) (*protocol.CodeLens, error) {
	return nil, nil
}
func (s *mdlServer) ColorPresentation(ctx context.Context, params *protocol.ColorPresentationParams) ([]protocol.ColorPresentation, error) {
	return nil, nil
}
func (s *mdlServer) Declaration(ctx context.Context, params *protocol.DeclarationParams) ([]protocol.Location, error) {
	return nil, nil
}
func (s *mdlServer) DidChangeWatchedFiles(ctx context.Context, params *protocol.DidChangeWatchedFilesParams) error {
	return nil
}
func (s *mdlServer) DidChangeWorkspaceFolders(ctx context.Context, params *protocol.DidChangeWorkspaceFoldersParams) error {
	return nil
}
func (s *mdlServer) DocumentColor(ctx context.Context, params *protocol.DocumentColorParams) ([]protocol.ColorInformation, error) {
	return nil, nil
}
func (s *mdlServer) DocumentHighlight(ctx context.Context, params *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	return nil, nil
}
func (s *mdlServer) DocumentLink(ctx context.Context, params *protocol.DocumentLinkParams) ([]protocol.DocumentLink, error) {
	return nil, nil
}
func (s *mdlServer) DocumentLinkResolve(ctx context.Context, params *protocol.DocumentLink) (*protocol.DocumentLink, error) {
	return nil, nil
}
func (s *mdlServer) ExecuteCommand(ctx context.Context, params *protocol.ExecuteCommandParams) (any, error) {
	return nil, nil
}
func (s *mdlServer) Formatting(ctx context.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (s *mdlServer) Implementation(ctx context.Context, params *protocol.ImplementationParams) ([]protocol.Location, error) {
	return nil, nil
}
func (s *mdlServer) OnTypeFormatting(ctx context.Context, params *protocol.DocumentOnTypeFormattingParams) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (s *mdlServer) PrepareRename(ctx context.Context, params *protocol.PrepareRenameParams) (*protocol.Range, error) {
	return nil, nil
}
func (s *mdlServer) RangeFormatting(ctx context.Context, params *protocol.DocumentRangeFormattingParams) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (s *mdlServer) References(ctx context.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	return nil, nil
}
func (s *mdlServer) Rename(ctx context.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (s *mdlServer) SignatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	return nil, nil
}
func (s *mdlServer) Symbols(ctx context.Context, params *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	return nil, nil
}
func (s *mdlServer) TypeDefinition(ctx context.Context, params *protocol.TypeDefinitionParams) ([]protocol.Location, error) {
	return nil, nil
}
func (s *mdlServer) WillSave(ctx context.Context, params *protocol.WillSaveTextDocumentParams) error {
	return nil
}
func (s *mdlServer) WillSaveWaitUntil(ctx context.Context, params *protocol.WillSaveTextDocumentParams) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (s *mdlServer) ShowDocument(ctx context.Context, params *protocol.ShowDocumentParams) (*protocol.ShowDocumentResult, error) {
	return nil, nil
}
func (s *mdlServer) WillCreateFiles(ctx context.Context, params *protocol.CreateFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (s *mdlServer) DidCreateFiles(ctx context.Context, params *protocol.CreateFilesParams) error {
	return nil
}
func (s *mdlServer) WillRenameFiles(ctx context.Context, params *protocol.RenameFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (s *mdlServer) DidRenameFiles(ctx context.Context, params *protocol.RenameFilesParams) error {
	return nil
}
func (s *mdlServer) WillDeleteFiles(ctx context.Context, params *protocol.DeleteFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (s *mdlServer) DidDeleteFiles(ctx context.Context, params *protocol.DeleteFilesParams) error {
	return nil
}
func (s *mdlServer) CodeLensRefresh(ctx context.Context) error { return nil }
func (s *mdlServer) PrepareCallHierarchy(ctx context.Context, params *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	return nil, nil
}
func (s *mdlServer) IncomingCalls(ctx context.Context, params *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	return nil, nil
}
func (s *mdlServer) OutgoingCalls(ctx context.Context, params *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	return nil, nil
}
func (s *mdlServer) SemanticTokensFull(ctx context.Context, params *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	return nil, nil
}
func (s *mdlServer) SemanticTokensFullDelta(ctx context.Context, params *protocol.SemanticTokensDeltaParams) (any, error) {
	return nil, nil
}
func (s *mdlServer) SemanticTokensRange(ctx context.Context, params *protocol.SemanticTokensRangeParams) (*protocol.SemanticTokens, error) {
	return nil, nil
}
func (s *mdlServer) SemanticTokensRefresh(ctx context.Context) error { return nil }
func (s *mdlServer) LinkedEditingRange(ctx context.Context, params *protocol.LinkedEditingRangeParams) (*protocol.LinkedEditingRanges, error) {
	return nil, nil
}
func (s *mdlServer) Moniker(ctx context.Context, params *protocol.MonikerParams) ([]protocol.Moniker, error) {
	return nil, nil
}
func (s *mdlServer) Request(ctx context.Context, method string, params any) (any, error) {
	return nil, nil
}
