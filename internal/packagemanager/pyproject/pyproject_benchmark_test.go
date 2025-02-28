package pyproject

import (
	"os"
	"testing"
)

// BenchmarkLoadAndUpdate benchmarks loading and updating a pyproject.toml file
func BenchmarkLoadAndUpdate(b *testing.B) {
	// Create a test pyproject.toml file with various dependencies
	content := `[project]
name = "benchmark-project"
version = "0.1.0"
description = "A project for benchmarking ru"
readme = "README.md"
requires-python = ">=3.13"
dependencies = [
    "requests==2.31.0",
    "flask==2.0.0",
    "django==4.0.0",
    "numpy>=1.20.0",
    "pandas==1.4.0",
    "matplotlib>=3.5.0",
    "scikit-learn==1.0.0",
    "tensorflow>=2.7.0",
    "pytorch>=1.10.0",
    "transformers==4.16.0"
]

[project.optional-dependencies]
dev = [
    "pytest==7.0.0",
    "black==22.1.0",
    "flake8>=4.0.0",
    "isort==5.10.0",
    "mypy>=0.930"
]

web = [
    "fastapi>=0.70.0",
    "uvicorn>=0.15.0",
    "jinja2==3.0.3"
]

[tool.poetry]
name = "benchmark-project"
version = "0.1.0"

[tool.poetry.dependencies]
python = ">=3.13"
sqlalchemy = "^1.4.30"
alembic = ">=1.7.0"
celery = "^5.2.0"
redis = ">=4.1.0"

[tool.isort]
profile = "black"

[tool.ruff]
line-length = 120
`

	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "pyproject-benchmark*.toml")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// Write the test content
	if err := os.WriteFile(tmpfile.Name(), []byte(content), 0644); err != nil {
		b.Fatal(err)
	}

	// Prepare version map
	versions := map[string]string{
		"requests":     "2.32.0",
		"flask":        "2.3.0",
		"django":       "4.2.0",
		"numpy":        "1.25.0",
		"pandas":       "2.0.0",
		"matplotlib":   "3.7.0",
		"scikit-learn": "1.2.0",
		"tensorflow":   "2.12.0",
		"pytorch":      "2.0.0",
		"transformers": "4.28.0",
		"pytest":       "7.3.0",
		"black":        "23.3.0",
		"flake8":       "6.0.0",
		"isort":        "5.12.0",
		"mypy":         "1.2.0",
		"fastapi":      "0.95.0",
		"uvicorn":      "0.21.0",
		"jinja2":       "3.1.2",
		"sqlalchemy":   "2.0.0",
		"alembic":      "1.11.0",
		"celery":       "5.3.0",
		"redis":        "4.5.0",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create a new PyProject instance for each iteration
		p := NewPyProject(tmpfile.Name())
		_, err := p.LoadAndUpdate(versions)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUpdateDependencyString benchmarks updating a dependency string
func BenchmarkUpdateDependencyString(b *testing.B) {
	// Test cases
	testCases := []struct {
		name     string
		line     string
		versions map[string]string
	}{
		{
			name:     "Simple equality constraint",
			line:     "requests==2.28.0",
			versions: map[string]string{"requests": "2.32.0"},
		},
		{
			name:     "Range constraint",
			line:     "flask>=2.0.0,<3.0.0",
			versions: map[string]string{"flask": "2.3.0"},
		},
		{
			name:     "Approximate constraint",
			line:     "django~=4.0.0",
			versions: map[string]string{"django": "4.2.0"},
		},
		{
			name:     "Caret constraint",
			line:     "numpy^1.20.0",
			versions: map[string]string{"numpy": "1.25.0"},
		},
		{
			name:     "No constraint",
			line:     "pandas",
			versions: map[string]string{"pandas": "2.0.0"},
		},
	}

	p := &PyProject{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tc := testCases[i%len(testCases)]
		p.updateDependencyString(tc.line, tc.versions)
	}
}

// BenchmarkUpdateComplexConstraint benchmarks updating a complex constraint
func BenchmarkUpdateComplexConstraint(b *testing.B) {
	// Test cases
	testCases := []struct {
		name     string
		dep      string
		versions map[string]string
	}{
		{
			name:     "Equality in complex constraint",
			dep:      "requests>=2.28.0,==2.31.0",
			versions: map[string]string{"requests": "2.32.0"},
		},
		{
			name:     "Multiple constraints",
			dep:      "flask>=2.0.0,<3.0.0,!=2.1.0",
			versions: map[string]string{"flask": "2.3.0"},
		},
		{
			name:     "Simple constraint",
			dep:      "django>=4.0.0",
			versions: map[string]string{"django": "4.2.0"},
		},
		{
			name:     "No update needed",
			dep:      "numpy>=1.20.0,<2.0.0",
			versions: map[string]string{"numpy": "1.25.0"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tc := testCases[i%len(testCases)]
		depCopy := tc.dep
		updateComplexConstraint(&depCopy, tc.versions)
	}
}
