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
