package release

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var dockerTagRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func ValidateReleaseOnLine(releaseTag, prefix string, major, minor uint64) error {
	stripped := strings.TrimPrefix(releaseTag, prefix)
	ver, err := semver.NewVersion(stripped)
	if err != nil {
		return fmt.Errorf("invalid semver release tag %q: %w", releaseTag, err)
	}
	if ver.Major() != major || ver.Minor() != minor {
		return fmt.Errorf("release tag %q is not on manifest line %d.%d.x", releaseTag, major, minor)
	}
	if ver.Prerelease() != "" {
		return fmt.Errorf("release tag %q must be release-shaped (no pre-release suffix)", releaseTag)
	}
	if ver.Metadata() != "" {
		return fmt.Errorf("release tag %q must not contain build metadata (+…)", releaseTag)
	}
	return nil
}

func IsDockerTagSafe(tag string) bool {
	return dockerTagRE.MatchString(tag)
}

func FormatReleaseTag(prefix string, major, minor, patch uint64) string {
	return fmt.Sprintf("%s%d.%d.%d", prefix, major, minor, patch)
}

func ParseTagVersion(name, prefix string) (*semver.Version, error) {
	stripped := strings.TrimPrefix(name, prefix)
	return semver.NewVersion(stripped)
}
