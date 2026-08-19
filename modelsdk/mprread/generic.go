// SPDX-License-Identifier: Apache-2.0

package mprread

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// Unit pairs a decoded gen-typed element with its SQLite ContainerID.
// ContainerID is a DB-level column that is not embedded in the BSON content,
// so callers that need it (e.g. building module-level trees) can obtain it
// here without a second query.
type Unit[T element.Element] struct {
	Element     T
	ContainerID model.ID
}

// bsonTypeName returns the canonical BSON $Type name for the type parameter T
// by looking up the Go concrete type in the codec DefaultRegistry reverse map.
// It panics at startup if T is not registered, which catches configuration
// errors at init time rather than silently returning empty results at runtime.
func bsonTypeName[T element.Element]() string {
	var zero T
	goType := reflect.TypeOf(&zero).Elem()
	name, ok := codec.DefaultRegistry.TypeNameOf(goType)
	if !ok {
		panic(fmt.Sprintf("mprread: type %v not registered in codec.DefaultRegistry — import the gen package", goType))
	}
	return name
}

// ListUnitsByType decodes every unit of type T and returns them as []T.
// The BSON $Type name is inferred from T via the codec registry — no magic
// string needed. Import the relevant gen/* package to trigger registration.
//
// ContainerID is not included; use ListUnitsWithContainer if you need it.
func ListUnitsByType[T element.Element](r *mmpr.Reader) ([]T, error) {
	units, err := ListUnitsWithContainer[T](r)
	if err != nil {
		return nil, err
	}
	out := make([]T, len(units))
	for i, u := range units {
		out[i] = u.Element
	}
	return out, nil
}

// ListUnitsWithContainer decodes every unit of type T and returns each element
// paired with its ContainerID. Uses ref.Contents from the unit index (already
// loaded by ListUnitsByType on the Reader) to avoid a second per-unit read.
// The BSON $Type name is inferred from T via the codec registry.
func ListUnitsWithContainer[T element.Element](r *mmpr.Reader) ([]Unit[T], error) {
	typeName := bsonTypeName[T]()
	refs, err := r.ListUnitsByType(typeName)
	if err != nil {
		return nil, err
	}
	dec := codec.NewDecoder(codec.DefaultRegistry)
	out := make([]Unit[T], 0, len(refs))
	for _, ref := range refs {
		if ref.Type != typeName {
			continue
		}
		elem, err := dec.Decode(bson.Raw(ref.Contents))
		if err != nil {
			return nil, fmt.Errorf("decode unit %s: %w", ref.ID, err)
		}
		typed, ok := elem.(T)
		if !ok {
			return nil, fmt.Errorf("unit %s decoded as %T, want %T", ref.ID, elem, *new(T))
		}
		out = append(out, Unit[T]{Element: typed, ContainerID: model.ID(ref.ContainerID)})
	}
	return out, nil
}

// GetUnitByName decodes the single unit of type T with the given qualified name,
// resolved through the Reader's lightweight BSON-header index. Returns
// (nil, nil) when no such document exists.
//
// This is the by-name counterpart to ListUnitsWithContainer: it derives the BSON
// $Type from the codec registry the same way, so it works for every registered
// type without an entry in mpr.rawUnitBSONType's hand-maintained alias table.
//
// Prefer this over "list every unit of the type and scan for the name" — the
// scan re-decodes the whole type on every call, which is what made DESCRIBE over
// a whole project quadratic (issue #651).
func GetUnitByName[T element.Element](r *mmpr.Reader, qualifiedName string) (*Unit[T], error) {
	typeName := bsonTypeName[T]()
	ref, err := r.GetUnitByTypeName(typeName, qualifiedName)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, nil
	}
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(ref.Contents))
	if err != nil {
		return nil, fmt.Errorf("decode unit %s (%s): %w", qualifiedName, ref.ID, err)
	}
	typed, ok := elem.(T)
	if !ok {
		return nil, fmt.Errorf("unit %s decoded as %T, want %T", ref.ID, elem, *new(T))
	}
	return &Unit[T]{Element: typed, ContainerID: model.ID(ref.ContainerID)}, nil
}

// ListUnitsWithContainerCached is ListUnitsWithContainer with the decoded result
// memoized on the Reader until the next InvalidateCache (i.e. until any write).
// A DESCRIBE sweep over a whole project calls the same List* repeatedly — once
// per document described — so without this the type is re-decoded N times.
//
// Two rules govern which call sites may use it:
//
//  1. Read-only callers only. The returned elements are shared with every later
//     caller, so mutating one in place (as the OQL rewrite in move_view_write.go
//     does) would poison the memo for readers that follow. Write paths must keep
//     using ListUnitsWithContainer, which decodes fresh.
//
//  2. Not for types whose decoded form is large. Measured on a 10,001-microflow
//     project: retaining the decoded microflows costs ~554MB, against a ~340MB
//     floor for the raw unit cache — every other document type measured stayed
//     at the floor, because its gen→model conversion keeps only header fields.
//     Flow types (Microflow, Nanoflow, Rule) resolve by name through
//     GetUnitByName[T] instead, which decodes exactly the one document asked for.
func ListUnitsWithContainerCached[T element.Element](r *mmpr.Reader) ([]Unit[T], error) {
	if decodeCacheDisabled() {
		return ListUnitsWithContainer[T](r)
	}
	typeName := bsonTypeName[T]()
	if v, ok := r.DecodedUnits(typeName); ok {
		if hit, ok := v.([]Unit[T]); ok {
			return hit, nil
		}
	}
	out, err := ListUnitsWithContainer[T](r)
	if err != nil {
		return nil, err
	}
	r.SetDecodedUnits(typeName, out)
	return out, nil
}

// decodeCacheDisabled reports whether MXCLI_NO_DECODE_CACHE turns the memo off.
//
// The memo trades peak memory for time: on a 10k-microflow, 23k-unit app,
// DESCRIBE over a whole module went 11.1s → 2.8s while peak RSS rose ~180MB
// (~16%), because decodes that used to be transient garbage are now retained
// until the next write. That is the right default, but a memory-constrained
// machine with a very large project can opt out, and a bisect of a suspected
// stale-read can rule the memo out in one run.
func decodeCacheDisabled() bool {
	v := strings.TrimSpace(os.Getenv("MXCLI_NO_DECODE_CACHE"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}
