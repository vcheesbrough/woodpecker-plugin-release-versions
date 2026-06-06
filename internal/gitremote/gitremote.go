package gitremote

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/release"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

func ListRemoteTags(cfg types.Config) ([]types.RemoteTag, error) {
	url := fmt.Sprintf("https://x-token:%s@github.com/%s.git", cfg.GitHubToken, cfg.Repo)
	out, err := exec.Command("git", "ls-remote", "--tags", "--refs", url).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git ls-remote failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return parseLsRemote(string(out)), nil
}

func TagForCommit(tags []types.RemoteTag, commitSHA string) string {
	short := commitSHA
	if len(short) > 7 {
		short = short[:7]
	}
	for _, tag := range tags {
		if tag.SHA == commitSHA || strings.HasPrefix(tag.SHA, short) {
			return tag.Name
		}
	}
	return ""
}

func NextPatch(tags []types.RemoteTag, major, minor uint64, prefix string) uint64 {
	var maxPatch *uint64
	for _, tag := range tags {
		ver, err := release.ParseTagVersion(tag.Name, prefix)
		if err != nil || ver.Prerelease() != "" {
			continue
		}
		if ver.Major() != major || ver.Minor() != minor {
			continue
		}
		patch := ver.Patch()
		if maxPatch == nil || patch > *maxPatch {
			maxPatch = &patch
		}
	}
	if maxPatch == nil {
		return 0
	}
	return *maxPatch + 1
}

func parseLsRemote(stdout string) []types.RemoteTag {
	peeled := map[string]string{}
	lightweight := map[string]string{}

	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}
		sha, ref := parts[0], parts[1]
		if !strings.HasPrefix(ref, "refs/tags/") {
			continue
		}
		name := strings.TrimPrefix(ref, "refs/tags/")
		if peeledName, ok := strings.CutSuffix(name, "^{}"); ok {
			peeled[peeledName] = sha
			continue
		}
		lightweight[name] = sha
	}

	out := make([]types.RemoteTag, 0, len(lightweight))
	for name, sha := range lightweight {
		if resolved, ok := peeled[name]; ok {
			sha = resolved
		}
		out = append(out, types.RemoteTag{Name: name, SHA: sha})
	}
	return out
}

func parseLsRemoteForTest(stdout string) []types.RemoteTag {
	return parseLsRemote(stdout)
}
