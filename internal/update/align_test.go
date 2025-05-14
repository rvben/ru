package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runAlignerTest(t *testing.T, tmpDir string) func(files []string, expected map[string]string) {
	return func(files []string, expected map[string]string) {
		aligner := NewAligner()
		if err := aligner.collectFilesToProcess(tmpDir); err != nil {
			t.Fatalf("collectFilesToProcess failed: %v", err)
		}
		if err := aligner.collectVersionsFromFiles(); err != nil {
			t.Fatalf("collectVersionsFromFiles failed: %v", err)
		}
		if err := aligner.alignVersionsInFiles(); err != nil {
			t.Fatalf("alignVersionsInFiles failed: %v", err)
		}
		for _, f := range files {
			updated, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", f, err)
			}
			str := string(updated)
			for _, want := range expected {
				if !strings.Contains(str, want) {
					t.Errorf("Expected %s in %s, got: %q", want, f, str)
				}
			}
		}
	}
}

func TestAlignerWildcardVersion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aligner-wildcard-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create three subdirectories, each with a requirements.txt
	dirs := []string{"a", "b", "c"}
	contents := []string{"rdflib==7.0.*\n", "rdflib==7.1.0\n", "rdflib==7.1.*\n"}
	files := make([]string, 3)
	for i, d := range dirs {
		dirPath := filepath.Join(tmpDir, d)
		if err := os.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dirPath, err)
		}
		filePath := filepath.Join(dirPath, "requirements.txt")
		if err := os.WriteFile(filePath, []byte(contents[i]), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", filePath, err)
		}
		files[i] = filePath
	}

	// Only expect concrete version
	runAlignerTest(t, tmpDir)(files, map[string]string{"rdflib": "==7.1.0"})
}

func TestAlignerComplexVersions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aligner-complex-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dirs := []string{"a", "b", "c", "d", "e"}
	contents := []string{
		"foo==1.0.0\nbar==2.0.0a1\nbaz==3.0.0.post1\nqux==4.0.*\n",
		"foo==1.2.0rc1\nbar==2.0.0\nbaz==3.0.0\nqux==4.1.0\n",
		"foo==1.1.0\nbar==2.0.0b2\nbaz==3.0.0.post2\nqux==4.1.*\n",
		"foo==1.2.0\nbar==2.0.0rc2\nbaz==3.1.0\nqux==4.2.0\n",
		"foo==1.2.0a1\nbar==2.0.0\nbaz==3.1.0.post1\nqux==4.2.*\n",
	}
	files := make([]string, len(dirs))
	for i, d := range dirs {
		dirPath := filepath.Join(tmpDir, d)
		if err := os.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dirPath, err)
		}
		filePath := filepath.Join(dirPath, "requirements.txt")
		if err := os.WriteFile(filePath, []byte(contents[i]), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", filePath, err)
		}
		files[i] = filePath
	}

	// Only expect concrete versions
	runAlignerTest(t, tmpDir)(files, map[string]string{
		"foo": "foo==1.2.0",
		"bar": "bar==2.0.0",
		"baz": "baz==3.1.0.post1",
		"qux": "qux==4.2.0",
	})
}

func TestAlignerUrllib3VersionOrder(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aligner-urllib3-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	files := []struct {
		dir, content string
	}{
		{"a", "urllib3==1.26.15\n"},
		{"b", "urllib3==2.2.3\n"},
	}
	paths := make([]string, len(files))
	for i, f := range files {
		dirPath := filepath.Join(tmpDir, f.dir)
		if err := os.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dirPath, err)
		}
		filePath := filepath.Join(dirPath, "requirements.txt")
		if err := os.WriteFile(filePath, []byte(f.content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", filePath, err)
		}
		paths[i] = filePath
	}

	// Only expect highest concrete version
	runAlignerTest(t, tmpDir)(paths, map[string]string{"urllib3": "urllib3==2.2.3"})
}
