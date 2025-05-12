package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	aligner := NewAligner()
	if err := aligner.collectVersions(tmpDir); err != nil {
		t.Fatalf("collectVersions failed: %v", err)
	}
	if err := aligner.alignVersions(tmpDir); err != nil {
		t.Fatalf("alignVersions failed: %v", err)
	}

	for _, f := range files {
		updated, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", f, err)
		}
		str := string(updated)
		if strings.Contains(str, "==7.1.") && !strings.Contains(str, "==7.1.*") {
			t.Errorf("Invalid version '==7.1.' found in %s: %q", f, str)
		}
		if !strings.Contains(str, "==7.1.0") && !strings.Contains(str, "==7.0.*") && !strings.Contains(str, "==7.1.*") {
			t.Errorf("Expected wildcard or valid version in %s, got: %q", f, str)
		}
	}

	// All files should be aligned to the highest valid version, which is 7.1.*
	for _, f := range files {
		updated, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", f, err)
		}
		str := string(updated)
		if !strings.Contains(str, "==7.1.*") {
			t.Errorf("Expected all files to be aligned to '==7.1.*', got: %q in %s", str, f)
		}
	}
}

func TestAlignerComplexVersions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aligner-complex-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create subdirectories with requirements.txt files containing complex version patterns
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

	aligner := NewAligner()
	if err := aligner.collectVersions(tmpDir); err != nil {
		t.Fatalf("collectVersions failed: %v", err)
	}
	if err := aligner.alignVersions(tmpDir); err != nil {
		t.Fatalf("alignVersions failed: %v", err)
	}

	// Expected highest versions:
	// foo: 1.2.0 (stable beats rc, a, etc.)
	// bar: 2.0.0 (stable beats rc, a, b)
	// baz: 3.1.0.post1 (post-release beats 3.1.0, 3.0.0.post2, 3.0.0.post1, 3.0.0)
	// qux: 4.2.* (wildcard beats 4.2.0, 4.1.*, 4.1.0, 4.0.*)

	for _, f := range files {
		updated, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", f, err)
		}
		str := string(updated)
		if !strings.Contains(str, "foo==1.2.0") {
			t.Errorf("Expected foo==1.2.0 in %s, got: %q", f, str)
		}
		if !strings.Contains(str, "bar==2.0.0") {
			t.Errorf("Expected bar==2.0.0 in %s, got: %q", f, str)
		}
		if !strings.Contains(str, "baz==3.1.0.post1") {
			t.Errorf("Expected baz==3.1.0.post1 in %s, got: %q", f, str)
		}
		if !strings.Contains(str, "qux==4.2.*") {
			t.Errorf("Expected qux==4.2.* in %s, got: %q", f, str)
		}
	}
}
