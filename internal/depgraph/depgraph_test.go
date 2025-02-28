package depgraph

import (
	"reflect"
	"testing"
)

func TestDetectCycles(t *testing.T) {
	tests := []struct {
		name           string
		dependencies   map[string]map[string]string // from -> to -> constraint
		expectedCycles [][]string
	}{
		{
			name: "simple cycle",
			dependencies: map[string]map[string]string{
				"A": {"B": ">=1.0.0"},
				"B": {"A": ">=1.0.0"},
			},
			expectedCycles: [][]string{
				{"A", "B", "A"},
			},
		},
		{
			name: "no cycles",
			dependencies: map[string]map[string]string{
				"A": {"B": ">=1.0.0"},
				"B": {"C": ">=1.0.0"},
				"C": {},
			},
			expectedCycles: nil,
		},
		{
			name: "complex cycle",
			dependencies: map[string]map[string]string{
				"A": {"B": ">=1.0.0"},
				"B": {"C": ">=1.0.0"},
				"C": {"D": ">=1.0.0"},
				"D": {"B": ">=1.0.0"},
			},
			expectedCycles: [][]string{
				{"B", "C", "D", "B"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New()
			for from, deps := range tt.dependencies {
				for to, constraint := range deps {
					if err := g.AddDependency(from, to, constraint); err != nil {
						t.Fatalf("Failed to add dependency: %v", err)
					}
				}
			}

			cycles := g.DetectCycles()
			if !reflect.DeepEqual(cycles, tt.expectedCycles) {
				t.Errorf("DetectCycles() = %v, want %v", cycles, tt.expectedCycles)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	tests := []struct {
		name          string
		dependencies  map[string]map[string]string
		updatePackage string
		newVersion    string
		expectError   bool
	}{
		{
			name: "valid update",
			dependencies: map[string]map[string]string{
				"A": {"B": ">=1.0.0"},
				"C": {"B": ">=1.0.0,<3.0.0"},
			},
			updatePackage: "B",
			newVersion:    "2.0.0",
			expectError:   false,
		},
		{
			name: "invalid update - violates constraint",
			dependencies: map[string]map[string]string{
				"A": {"B": ">=1.0.0,<2.0.0"},
				"C": {"B": ">=1.0.0,<3.0.0"},
			},
			updatePackage: "B",
			newVersion:    "2.0.0",
			expectError:   true,
		},
		{
			name: "compatible release operator",
			dependencies: map[string]map[string]string{
				"A": {"B": "~=2.2"},
			},
			updatePackage: "B",
			newVersion:    "2.9.0",
			expectError:   false,
		},
		{
			name: "compatible release violation",
			dependencies: map[string]map[string]string{
				"A": {"B": "~=2.2"},
			},
			updatePackage: "B",
			newVersion:    "3.0.0",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New()
			for from, deps := range tt.dependencies {
				for to, constraint := range deps {
					if err := g.AddDependency(from, to, constraint); err != nil {
						t.Fatalf("Failed to add dependency: %v", err)
					}
				}
			}

			err := g.ValidateUpdate(tt.updatePackage, tt.newVersion)
			if (err != nil) != tt.expectError {
				t.Errorf("ValidateUpdate() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestGetUpdateOrder(t *testing.T) {
	tests := []struct {
		name          string
		dependencies  map[string]map[string]string
		expectedOrder []string
	}{
		{
			name: "simple chain",
			dependencies: map[string]map[string]string{
				"A": {},
				"B": {"A": ">=1.0.0"},
				"C": {"B": ">=1.0.0"},
			},
			expectedOrder: []string{"A", "B", "C"},
		},
		{
			name: "diamond dependency",
			dependencies: map[string]map[string]string{
				"A": {},
				"B": {"A": ">=1.0.0"},
				"C": {"A": ">=1.0.0"},
				"D": {"B": ">=1.0.0", "C": ">=1.0.0"},
			},
			expectedOrder: []string{"A", "B", "C", "D"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New()
			for from, deps := range tt.dependencies {
				for to, constraint := range deps {
					if err := g.AddDependency(from, to, constraint); err != nil {
						t.Fatalf("Failed to add dependency: %v", err)
					}
				}
			}

			order := g.GetUpdateOrder()
			if !reflect.DeepEqual(order, tt.expectedOrder) {
				t.Errorf("GetUpdateOrder() = %v, want %v", order, tt.expectedOrder)
			}
		})
	}
}
