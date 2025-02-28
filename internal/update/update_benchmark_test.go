package update

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"
)

// MockBenchPyPI implements the PackageManager interface for benchmarking
type MockBenchPyPI struct {
	versions map[string]string
}

func (m *MockBenchPyPI) GetLatestVersion(pkg string) (string, error) {
	if version, ok := m.versions[pkg]; ok {
		return version, nil
	}
	return "1.0.0", nil // Default version for benchmarking
}

func (m *MockBenchPyPI) SetCustomIndexURL() error {
	return nil
}

// BenchmarkCheckVersionConstraints benchmarks the version constraint checking
func BenchmarkCheckVersionConstraints(b *testing.B) {
	// Prepare test data
	constraints := []string{
		"==1.0.0",
		">=2.0.0",
		"<=3.0.0",
		">1.0.0",
		"<3.0.0",
		"~=2.0.0",
		"^1.0.0",
		">=1.0.0,<2.0.0",
		">=1.0.0,<=2.0.0,!=1.5.0",
		"1.0.0",
	}
	latestVersions := []string{
		"1.1.0",
		"2.0.0",
		"3.0.0",
		"2.3.4",
		"1.9.9",
	}

	updater := New(true, false, nil)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		constraint := constraints[i%len(constraints)]
		latestVer := latestVersions[i%len(latestVersions)]
		_, _ = updater.checkVersionConstraints(latestVer, constraint)
	}
}

// BenchmarkUpdateRequirementsFile benchmarks updating a requirements file
func BenchmarkUpdateRequirementsFile(b *testing.B) {
	// Create a mock requirements file with various dependencies
	content := `# Sample requirements file for benchmarking
requests==2.28.0
flask>=2.0.0,<3.0.0
django==4.0.0
numpy>=1.20.0
pandas==1.4.0
matplotlib>=3.5.0
scikit-learn==1.0.0
tensorflow>=2.7.0
pytorch>=1.10.0
transformers==4.16.0
fastapi>=0.70.0
uvicorn>=0.15.0
pytest==7.0.0
black==22.1.0
flake8>=4.0.0
isort==5.10.0
mypy>=0.930
sqlalchemy==1.4.30
alembic>=1.7.0
celery==5.2.0
redis>=4.1.0
`

	b.ResetTimer()
	b.StopTimer()

	updater := New(true, false, nil)
	updater.pypi = &MockBenchPyPI{
		versions: map[string]string{
			"requests":     "2.32.0",
			"flask":        "2.2.0",
			"django":       "4.2.0",
			"numpy":        "1.25.0",
			"pandas":       "2.0.0",
			"matplotlib":   "3.7.0",
			"scikit-learn": "1.2.0",
			"tensorflow":   "2.12.0",
			"pytorch":      "2.0.0",
			"transformers": "4.28.0",
			"fastapi":      "0.95.0",
			"uvicorn":      "0.21.0",
			"pytest":       "7.3.0",
			"black":        "23.3.0",
			"flake8":       "6.0.0",
			"isort":        "5.12.0",
			"mypy":         "1.2.0",
			"sqlalchemy":   "2.0.0",
			"alembic":      "1.11.0",
			"celery":       "5.3.0",
			"redis":        "4.5.0",
		},
	}

	for i := 0; i < b.N; i++ {
		// Create a temporary file for each iteration
		tempDir, err := ioutil.TempDir("", "ru-benchmark-")
		if err != nil {
			b.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		requirementsFile := tempDir + "/requirements.txt"
		if err := ioutil.WriteFile(requirementsFile, []byte(content), 0644); err != nil {
			b.Fatal(err)
		}

		b.StartTimer()
		err = updater.updateRequirementsFile(requirementsFile)
		b.StopTimer()

		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProcessSmallRequirementsFiles benchmarks processing a small number of requirements files
func BenchmarkProcessSmallRequirementsFiles(b *testing.B) {
	// Create a temporary directory with a few requirements files
	tempDir, err := ioutil.TempDir("", "ru-benchmark-small-")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a few requirements files with various dependencies
	files := []struct {
		name    string
		content string
	}{
		{
			name: "requirements.txt",
			content: `requests==2.28.0
flask==2.0.0
django==4.0.0
`,
		},
		{
			name: "requirements-dev.txt",
			content: `pytest==7.0.0
black==22.1.0
flake8==4.0.0
`,
		},
		{
			name: "requirements-prod.txt",
			content: `gunicorn==20.1.0
uvicorn==0.15.0
fastapi==0.70.0
`,
		},
	}

	for _, file := range files {
		filePath := tempDir + "/" + file.name
		if err := ioutil.WriteFile(filePath, []byte(file.content), 0644); err != nil {
			b.Fatal(err)
		}
	}

	updater := New(true, false, []string{tempDir})
	updater.pypi = &MockBenchPyPI{
		versions: map[string]string{
			"requests": "2.32.0",
			"flask":    "2.3.0",
			"django":   "4.2.0",
			"pytest":   "7.3.0",
			"black":    "23.3.0",
			"flake8":   "6.0.0",
			"gunicorn": "21.0.0",
			"uvicorn":  "0.21.0",
			"fastapi":  "0.95.0",
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := updater.ProcessDirectory(tempDir); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProcessLargeRequirementsFiles benchmarks processing a large number of requirements files
func BenchmarkProcessLargeRequirementsFiles(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping large benchmark in short mode")
	}

	// Create a temporary directory with many requirements files
	tempDir, err := ioutil.TempDir("", "ru-benchmark-large-")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create 50 requirements files with various dependencies
	baseContent := `# Sample requirements file for benchmarking
requests==2.28.0
flask>=2.0.0,<3.0.0
django==4.0.0
numpy>=1.20.0
pandas==1.4.0
matplotlib>=3.5.0
scikit-learn==1.0.0
tensorflow>=2.7.0
pytorch>=1.10.0
transformers==4.16.0
`

	for i := 0; i < 50; i++ {
		char := string(rune('a' + i%26))
		num := strconv.Itoa(i / 26)
		fileName := tempDir + "/requirements-" + char + num + ".txt"
		if err := ioutil.WriteFile(fileName, []byte(baseContent), 0644); err != nil {
			b.Fatal(err)
		}
	}

	updater := New(true, false, []string{tempDir})
	updater.pypi = &MockBenchPyPI{
		versions: map[string]string{
			"requests":     "2.32.0",
			"flask":        "2.2.0",
			"django":       "4.2.0",
			"numpy":        "1.25.0",
			"pandas":       "2.0.0",
			"matplotlib":   "3.7.0",
			"scikit-learn": "1.2.0",
			"tensorflow":   "2.12.0",
			"pytorch":      "2.0.0",
			"transformers": "4.28.0",
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := updater.ProcessDirectory(tempDir); err != nil {
			b.Fatal(err)
		}
	}
}
