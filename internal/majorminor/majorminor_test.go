package majorminor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCargoWorkspaceVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml")
	if err := os.WriteFile(path, []byte(`
[workspace.package]
version = "1.23.0"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	major, minor, err := Read("cargo", path)
	if err != nil {
		t.Fatal(err)
	}
	if major != 1 || minor != 23 {
		t.Fatalf("got %d.%d", major, minor)
	}
}

func TestReadGoVersionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "version.go")
	if err := os.WriteFile(path, []byte(`package version

const Version = "0.1.0"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	major, minor, err := Read("go", path)
	if err != nil {
		t.Fatal(err)
	}
	if major != 0 || minor != 1 {
		t.Fatalf("got %d.%d", major, minor)
	}
}

func TestReadGoVersionRejectsPrerelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "version.go")
	if err := os.WriteFile(path, []byte(`const Version = "1.0.0-beta"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read("go", path); err == nil {
		t.Fatal("expected error for pre-release version")
	}
}

func TestReadPyproject(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"pep621": "[project]\nname = \"code-vitals\"\nversion = \"0.4.0\"\n",
		"poetry": "[tool.poetry]\nname = \"code-vitals\"\nversion = \"0.4.0\"\n",
	} {
		path := filepath.Join(dir, name+".toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		major, minor, err := Read("pyproject", path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if major != 0 || minor != 4 {
			t.Fatalf("%s: got %d.%d", name, major, minor)
		}
	}
}

func TestReadPyprojectWithoutVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pyproject.toml")
	if err := os.WriteFile(path, []byte("[project]\nname = \"x\"\ndynamic = [\"version\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read("pyproject", path); err == nil {
		t.Fatal("expected an error for a dynamic version")
	}
}
