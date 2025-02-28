package main

import (
	"flag"
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"

	"github.com/rvben/ru/internal/update"
	"github.com/rvben/ru/internal/utils"
)

// runUpdateCommand simulates running the "update" command with the given arguments
func runUpdateCommand(args []string) error {
	// Create a new FlagSet for update command
	updateFlags := flag.NewFlagSet("update", flag.ExitOnError)
	verifyFlag := updateFlags.Bool("verify", false, "Verify dependency compatibility (slower)")
	noCacheFlag := updateFlags.Bool("no-cache", false, "Disable caching")
	dirFlag := updateFlags.String("dir", "", "Directory to process")
	verboseFlag := updateFlags.Bool("verbose", false, "Enable verbose logging")

	// Parse flags
	if err := updateFlags.Parse(args[1:]); err != nil {
		return err
	}

	// Set verbose mode based on the flag value
	utils.SetVerbose(*verboseFlag)

	// Get paths - either from the dir flag or from the remaining arguments
	var paths []string
	if *dirFlag != "" {
		paths = []string{*dirFlag}
	} else {
		paths = updateFlags.Args()
	}

	// Create and run the updater
	updater := update.New(*noCacheFlag, *verifyFlag, paths)
	return updater.Run()
}

// TestProfileUpdate runs a full update on a mix of files and generates CPU and memory profiles
func TestProfileUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profile test in short mode")
	}

	// Create a temporary directory for profiling
	tempDir, err := os.MkdirTemp("", "ru-profile-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create sample files for profiling
	createProfileFiles(t, tempDir)

	// Create absolute profile paths
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cpuProfilePath := filepath.Join(testDir, "cpu.prof")
	memProfilePath := filepath.Join(testDir, "mem.prof")

	// Create CPU profile
	cpuProfile, err := os.Create(cpuProfilePath)
	if err != nil {
		t.Fatal(err)
	}
	defer cpuProfile.Close()

	// Start CPU profiling
	if err := pprof.StartCPUProfile(cpuProfile); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	// Run the actual update operation
	if err := runUpdateCommand([]string{"update", "-dir", tempDir, "-no-cache"}); err != nil {
		t.Fatal(err)
	}

	// Create memory profile
	memProfile, err := os.Create(memProfilePath)
	if err != nil {
		t.Fatal(err)
	}
	defer memProfile.Close()

	// Write memory profile
	if err := pprof.WriteHeapProfile(memProfile); err != nil {
		t.Fatal(err)
	}

	t.Logf("Profiles saved to: %s and %s", cpuProfilePath, memProfilePath)
}

// createProfileFiles creates a mix of requirements files for profiling
func createProfileFiles(t *testing.T, dir string) {
	// Create requirements.txt
	requirementsContent := `# Python dependencies
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
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(requirementsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create requirements-dev.txt
	devRequirementsContent := `# Development dependencies
pytest==7.0.0
black==22.1.0
flake8>=4.0.0
isort==5.10.0
mypy>=0.930
coverage==6.3.2
tox==3.24.5
pylint==2.12.2
bandit==1.7.0
sphinx==4.4.0
`
	if err := os.WriteFile(filepath.Join(dir, "requirements-dev.txt"), []byte(devRequirementsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create pyproject.toml
	pyprojectContent := `[project]
name = "profile-project"
version = "0.1.0"
description = "A project for profiling ru"
readme = "README.md"
requires-python = ">=3.13"
dependencies = [
    "requests==2.31.0",
    "flask==2.0.0",
    "django==4.0.0",
    "numpy>=1.20.0",
    "pandas==1.4.0"
]

[project.optional-dependencies]
dev = [
    "pytest==7.0.0",
    "black==22.1.0"
]

[tool.poetry]
name = "profile-project"
version = "0.1.0"

[tool.poetry.dependencies]
python = ">=3.13"
sqlalchemy = "^1.4.30"
alembic = ">=1.7.0"
`
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyprojectContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create package.json
	packageJSONContent := `{
  "name": "profile-project",
  "version": "1.0.0",
  "description": "A project for profiling ru",
  "main": "index.js",
  "dependencies": {
    "express": "^4.17.1",
    "react": "17.0.2",
    "react-dom": "17.0.2",
    "lodash": "~4.17.20",
    "axios": "0.21.1"
  },
  "devDependencies": {
    "jest": "26.6.3",
    "eslint": "^7.22.0",
    "prettier": "2.2.1",
    "typescript": "^4.2.3"
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSONContent), 0644); err != nil {
		t.Fatal(err)
	}
}
