// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

func TestFindEmptyContainers_PageWithEmpty(t *testing.T) {
	rawData := map[string]any{
		"FormCall": map[string]any{
			"Arguments": []any{
				map[string]any{
					"Widgets": []any{
						map[string]any{
							"$Type":   "Forms$DivContainer",
							"Name":    "emptyDiv",
							"Widgets": []any{},
						},
					},
				},
			},
		},
	}

	result := findEmptyContainers(rawData)
	if len(result) != 1 {
		t.Fatalf("expected 1 empty container, got %d", len(result))
	}
	if result[0].Name != "emptyDiv" {
		t.Errorf("expected name 'emptyDiv', got %q", result[0].Name)
	}
}

func TestFindEmptyContainers_PageWithChildren(t *testing.T) {
	rawData := map[string]any{
		"FormCall": map[string]any{
			"Arguments": []any{
				map[string]any{
					"Widgets": []any{
						map[string]any{
							"$Type": "Forms$DivContainer",
							"Name":  "filledDiv",
							"Widgets": []any{
								map[string]any{
									"$Type": "Forms$TextBox",
									"Name":  "txt1",
								},
							},
						},
					},
				},
			},
		},
	}

	result := findEmptyContainers(rawData)
	if len(result) != 0 {
		t.Errorf("expected 0 empty containers, got %d", len(result))
	}
}

func TestFindEmptyContainers_SnippetStructure(t *testing.T) {
	rawData := map[string]any{
		"Widgets": []any{
			map[string]any{
				"$Type":   "Forms$DivContainer",
				"Name":    "snippetEmpty",
				"Widgets": []any{},
			},
		},
	}

	result := findEmptyContainers(rawData)
	if len(result) != 1 {
		t.Fatalf("expected 1 empty container, got %d", len(result))
	}
	if result[0].Name != "snippetEmpty" {
		t.Errorf("expected name 'snippetEmpty', got %q", result[0].Name)
	}
}

func TestFindEmptyContainers_NestedInLayoutGrid(t *testing.T) {
	rawData := map[string]any{
		"FormCall": map[string]any{
			"Arguments": []any{
				map[string]any{
					"Widgets": []any{
						map[string]any{
							"$Type": "Forms$LayoutGrid",
							"Name":  "grid1",
							"Rows": []any{
								map[string]any{
									"Columns": []any{
										map[string]any{
											"Widgets": []any{
												map[string]any{
													"$Type":   "Forms$DivContainer",
													"Name":    "nestedEmpty",
													"Widgets": []any{},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result := findEmptyContainers(rawData)
	if len(result) != 1 {
		t.Fatalf("expected 1 empty container, got %d", len(result))
	}
	if result[0].Name != "nestedEmpty" {
		t.Errorf("expected name 'nestedEmpty', got %q", result[0].Name)
	}
}

func TestFindEmptyContainers_NestedInTabContainer(t *testing.T) {
	rawData := map[string]any{
		"FormCall": map[string]any{
			"Arguments": []any{
				map[string]any{
					"Widgets": []any{
						map[string]any{
							"$Type": "Forms$TabContainer",
							"Name":  "tabs",
							"TabPages": []any{
								map[string]any{
									"Widgets": []any{
										map[string]any{
											"$Type":   "Forms$DivContainer",
											"Name":    "tabEmpty",
											"Widgets": []any{},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result := findEmptyContainers(rawData)
	if len(result) != 1 {
		t.Fatalf("expected 1 empty container, got %d", len(result))
	}
	if result[0].Name != "tabEmpty" {
		t.Errorf("expected name 'tabEmpty', got %q", result[0].Name)
	}
}

func TestFindEmptyContainers_NonContainer(t *testing.T) {
	rawData := map[string]any{
		"FormCall": map[string]any{
			"Arguments": []any{
				map[string]any{
					"Widgets": []any{
						map[string]any{
							"$Type": "Forms$TextBox",
							"Name":  "txt1",
						},
					},
				},
			},
		},
	}

	result := findEmptyContainers(rawData)
	if len(result) != 0 {
		t.Errorf("expected 0 empty containers, got %d", len(result))
	}
}

func TestFindEmptyContainers_Multiple(t *testing.T) {
	rawData := map[string]any{
		"FormCall": map[string]any{
			"Arguments": []any{
				map[string]any{
					"Widgets": []any{
						map[string]any{
							"$Type":   "Forms$DivContainer",
							"Name":    "empty1",
							"Widgets": []any{},
						},
						map[string]any{
							"$Type":   "Forms$DivContainer",
							"Name":    "empty2",
							"Widgets": []any{},
						},
					},
				},
			},
		},
	}

	result := findEmptyContainers(rawData)
	if len(result) != 2 {
		t.Fatalf("expected 2 empty containers, got %d", len(result))
	}
}

func TestFindEmptyContainersRecursive_FooterWidgets(t *testing.T) {
	w := map[string]any{
		"$Type": "Forms$SomeContainer",
		"Name":  "outer",
		"FooterWidgets": []any{
			map[string]any{
				"$Type":   "Forms$DivContainer",
				"Name":    "footerEmpty",
				"Widgets": []any{},
			},
		},
	}

	result := findEmptyContainersRecursive(w)
	if len(result) != 1 {
		t.Fatalf("expected 1 empty container, got %d", len(result))
	}
	if result[0].Name != "footerEmpty" {
		t.Errorf("expected name 'footerEmpty', got %q", result[0].Name)
	}
}

func TestEmptyContainerRule_Metadata(t *testing.T) {
	r := NewEmptyContainerRule()
	if r.ID() != "MPR006" {
		t.Errorf("ID = %q, want MPR006", r.ID())
	}
	if r.Category() != "quality" {
		t.Errorf("Category = %q, want quality", r.Category())
	}
	if r.Name() != "EmptyContainer" {
		t.Errorf("Name = %q, want EmptyContainer", r.Name())
	}
	// ako/mxcli#969: an empty container was measured to build and render
	// (11.13.0, run --local, no console errors); the old "crashes at runtime"
	// warning had no evidence behind it.
	if r.DefaultSeverity() != linter.SeverityInfo {
		t.Errorf("DefaultSeverity = %v, want info", r.DefaultSeverity())
	}
	if strings.Contains(strings.ToLower(r.Description()), "crash") {
		t.Errorf("Description %q still claims a crash", r.Description())
	}
}
