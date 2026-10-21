package release

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

const (
	TagsPrefix      = "refs/tags/"
	BuildsPrefix    = "refs/builds/"
	PipelinesPrefix = "refs/build-pipelines/"
)

var dockerTagRE = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)

func IsDockerTagSafe(tag string) bool {
	return dockerTagRE.MatchString(tag)
}

func FormatReleaseTag(prefix string, major, minor, patch uint64) string {
	return fmt.Sprintf("%s%d.%d.%d", prefix, major, minor, patch)
}

// Parse accepts only release-shaped names: the prefix, then a strict
// MAJOR.MINOR.PATCH with no pre-release or build metadata.
func Parse(name, prefix string) (*semver.Version, bool) {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return nil, false
	}
	ver, err := semver.StrictNewVersion(rest)
	if err != nil || ver.Prerelease() != "" || ver.Metadata() != "" {
		return nil, false
	}
	return ver, true
}

// NextPatch is one past the highest patch on the line across release tags and
// build refs, so a number is never reused: not one a failed build burned, and
// not one an older scheme already tagged.
func NextPatch(refs []types.Ref, major, minor uint64, prefix string) uint64 {
	var next uint64
	for _, ref := range refs {
		name, ok := cutAny(ref.Name, TagsPrefix, BuildsPrefix)
		if !ok {
			continue
		}
		ver, ok := Parse(name, prefix)
		if !ok || ver.Major() != major || ver.Minor() != minor {
			continue
		}
		if ver.Patch()+1 > next {
			next = ver.Patch() + 1
		}
	}
	return next
}

// TagsForCommit returns the release tags on a commit.
func TagsForCommit(refs []types.Ref, commit, prefix string) []string {
	var out []string
	for _, ref := range refs {
		name, ok := strings.CutPrefix(ref.Name, TagsPrefix)
		if !ok || ref.Commit() != commit {
			continue
		}
		if _, ok := Parse(name, prefix); ok {
			out = append(out, name)
		}
	}
	return out
}

// TagExists reports whether refs/tags/<name> exists on the remote, on any commit.
func TagExists(refs []types.Ref, name string) bool {
	for _, ref := range refs {
		if ref.Name == TagsPrefix+name {
			return true
		}
	}
	return false
}

// BuildsForCommit returns the versions built from a commit, newest first.
func BuildsForCommit(refs []types.Ref, commit, prefix string) []string {
	var vers []*semver.Version
	names := map[*semver.Version]string{}
	for _, ref := range refs {
		name, ok := strings.CutPrefix(ref.Name, BuildsPrefix)
		if !ok || ref.Commit() != commit {
			continue
		}
		if ver, ok := Parse(name, prefix); ok {
			vers = append(vers, ver)
			names[ver] = name
		}
	}
	sort.Slice(vers, func(i, j int) bool { return vers[i].GreaterThan(vers[j]) })
	out := make([]string, len(vers))
	for i, v := range vers {
		out[i] = names[v]
	}
	return out
}

// BuildExists reports whether refs/builds/<name> exists on the remote.
func BuildExists(refs []types.Ref, name string) bool {
	for _, ref := range refs {
		if ref.Name == BuildsPrefix+name {
			return true
		}
	}
	return false
}

// PipelineRef is the marker that lets every workflow of one pipeline find the
// version the first of them claimed.
func PipelineRef(pipelineID, version string) string {
	return PipelinesPrefix + pipelineID + "/" + version
}

// PipelineVersion returns the version a pipeline already claimed on this line,
// or "" if none. If racing workflows claimed several, the lowest wins, so every
// workflow that lists after the claims agrees.
func PipelineVersion(refs []types.Ref, pipelineID string, major, minor uint64, prefix string) string {
	var best *semver.Version
	bestName := ""
	for _, ref := range refs {
		name, ok := strings.CutPrefix(ref.Name, PipelinesPrefix+pipelineID+"/")
		if !ok {
			continue
		}
		ver, ok := Parse(name, prefix)
		if !ok || ver.Major() != major || ver.Minor() != minor {
			continue
		}
		if best == nil || ver.LessThan(best) {
			best, bestName = ver, name
		}
	}
	return bestName
}

func cutAny(s string, prefixes ...string) (string, bool) {
	for _, p := range prefixes {
		if rest, ok := strings.CutPrefix(s, p); ok {
			return rest, true
		}
	}
	return "", false
}
