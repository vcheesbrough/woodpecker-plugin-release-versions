package majorminor

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/semver/v3"
)

var goVersionRE = regexp.MustCompile(`Version\s*=\s*"([^"]+)"`)

func Read(source, path string) (major, minor uint64, err error) {
	switch source {
	case "cargo":
		return readCargo(path)
	case "go":
		return readGoVersionFile(path)
	default:
		return 0, 0, fmt.Errorf("unsupported major_minor_source %q; supported: cargo, go", source)
	}
}

func readCargo(path string) (uint64, uint64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, fmt.Errorf("major_minor_file %s: not found", path)
	}
	var doc map[string]any
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return 0, 0, fmt.Errorf("%s: invalid TOML: %w", path, err)
	}
	versionStr, err := cargoVersionString(doc)
	if err != nil {
		return 0, 0, err
	}
	return parseMajorMinor(versionStr)
}

func cargoVersionString(doc map[string]any) (string, error) {
	if ws, ok := doc["workspace"].(map[string]any); ok {
		if pkg, ok := ws["package"].(map[string]any); ok {
			if v, ok := pkg["version"].(string); ok && v != "" {
				return v, nil
			}
		}
	}
	if pkg, ok := doc["package"].(map[string]any); ok {
		if v, ok := pkg["version"].(string); ok && v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("Cargo.toml: no [workspace.package].version or [package].version")
}

func readGoVersionFile(path string) (uint64, uint64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, fmt.Errorf("major_minor_file %s: not found", path)
	}
	match := goVersionRE.FindStringSubmatch(string(raw))
	if len(match) < 2 {
		return 0, 0, fmt.Errorf("%s: no Version = \"x.y.z\" constant found", path)
	}
	return parseMajorMinor(match[1])
}

func parseMajorMinor(version string) (uint64, uint64, error) {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	ver, err := semver.NewVersion(v)
	if err != nil {
		return 0, 0, fmt.Errorf("version %q is not valid semver: %w", version, err)
	}
	if ver.Prerelease() != "" {
		return 0, 0, fmt.Errorf("version %q must be release-shaped (MAJOR.MINOR.PATCH)", version)
	}
	return ver.Major(), ver.Minor(), nil
}

func Line(major, minor uint64) string {
	return fmt.Sprintf("%d.%d", major, minor)
}
