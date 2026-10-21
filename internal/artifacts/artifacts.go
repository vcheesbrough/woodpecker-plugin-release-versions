package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ReleaseTagFile = ".release-tag"

// WriteReleaseTag writes .release-tag, the one way the version travels between
// steps, and mirrors it to GitHub Actions step outputs when githubOutput is set.
func WriteReleaseTag(outputDir, releaseTag, githubOutput string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(outputDir, ReleaseTagFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(releaseTag+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if githubOutput == "" {
		return nil
	}
	f, err := os.OpenFile(githubOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "release_tag=%s\n", releaseTag)
	return err
}

func ReadReleaseTag(outputDir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(outputDir, ReleaseTagFile))
	if err != nil {
		return "", fmt.Errorf("missing %s in %s; run mode version (build) or resolve (deploy) first", ReleaseTagFile, outputDir)
	}
	tag := strings.TrimSpace(string(raw))
	if tag == "" {
		return "", fmt.Errorf("%s in %s is empty", ReleaseTagFile, outputDir)
	}
	return tag, nil
}
