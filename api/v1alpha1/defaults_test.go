// SPDX-License-Identifier: AGPL-3.0-only

package v1alpha1

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"testing"
)

// wantVolumeSizeDefaults maps "TypeName.FieldName" to the exported constant that must
// equal that field's "+kubebuilder:default={size:"…"}" marker. A kubebuilder marker
// cannot reference a Go identifier, so the two literals can only be kept in sync by a
// test like this one, not by the compiler.
var wantVolumeSizeDefaults = map[string]string{
	"StorageSpec.Data":        DefaultDataSize,
	"StorageSpec.Media":       DefaultMediaSize,
	"StorageSpec.Consume":     DefaultConsumeSize,
	"StorageSpec.Export":      DefaultExportSize,
	"ManagedDatabase.Storage": DefaultDatabaseSize,
}

var sizeDefaultMarker = regexp.MustCompile(`\+kubebuilder:default=\{size:"([^"]*)"\}`)

// TestVolumeSizeDefaultsMatchMarkers parses this package's source (not the generated
// CRD, so it runs before controller-gen has ever produced one) and fails if a
// "+kubebuilder:default={size:...}" marker and its Go constant disagree.
func TestVolumeSizeDefaultsMatchMarkers(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "paperlessinstance_types.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing paperlessinstance_types.go: %v", err)
	}

	found := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range st.Fields.List {
			if field.Doc == nil || len(field.Names) == 0 {
				continue
			}
			for _, c := range field.Doc.List {
				m := sizeDefaultMarker.FindStringSubmatch(c.Text)
				if m == nil {
					continue
				}
				found[ts.Name.Name+"."+field.Names[0].Name] = m[1]
			}
		}
		return true
	})

	for key, want := range wantVolumeSizeDefaults {
		got, ok := found[key]
		if !ok {
			t.Errorf("%s: no +kubebuilder:default={size:...} marker found in source", key)
			continue
		}
		if got != want {
			t.Errorf("%s: marker default %q does not match its constant %q", key, got, want)
		}
	}
	if len(found) != len(wantVolumeSizeDefaults) {
		t.Errorf("found %d size-default markers in source, want %d; wantVolumeSizeDefaults is missing an entry or a field was removed", len(found), len(wantVolumeSizeDefaults))
	}
}
