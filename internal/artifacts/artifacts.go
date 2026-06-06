package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteReleaseArtifacts(outputDir, releaseTag, majorMinor string, reused, dryRun bool) error {
	if dryRun {
		fmt.Printf(
			"dry-run: would write .release-tag=%s reused=%t major_minor=%s\n",
			releaseTag,
			reused,
			majorMinor,
		)
		return nil
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	reusedStr := "false"
	if reused {
		reusedStr = "true"
	}
	if err := writeAtomic(filepath.Join(outputDir, ".release-tag"), releaseTag); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(outputDir, ".release-tag-reused"), reusedStr); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(outputDir, ".release-tag-major-minor"), majorMinor)
}

func ReadReleaseTag(outputDir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(outputDir, ".release-tag"))
	if err != nil {
		return "", fmt.Errorf("missing .release-tag in %s; run compute first", outputDir)
	}
	tag := strings.TrimSpace(string(raw))
	if tag == "" {
		return "", fmt.Errorf(".release-tag in %s is empty", outputDir)
	}
	return tag, nil
}

func writeAtomic(path, contents string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(contents), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
