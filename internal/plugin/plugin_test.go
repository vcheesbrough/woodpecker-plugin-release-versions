package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/artifacts"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/registry"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/testgit"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

const image = "registry.example/app"

type fakeRegistry struct {
	mu     sync.Mutex
	images map[string]map[string]string
	err    error
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{images: map[string]map[string]string{}}
}

func (f *fakeRegistry) add(version, commit string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.images[image+":"+version] = map[string]string{registry.LabelVersion: version, registry.LabelRevision: commit}
}

func (f *fakeRegistry) Exists(ref string) (bool, error) {
	_, found, err := f.Labels(ref)
	return found, err
}

func (f *fakeRegistry) Labels(ref string) (map[string]string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, false, f.err
	}
	labels, ok := f.images[ref]
	return labels, ok, nil
}

type fakeDocker struct {
	local  map[string]map[string]string
	logins []string
	pushed []string
}

func (f *fakeDocker) Labels(ref string) (map[string]string, error) {
	labels, ok := f.local[ref]
	if !ok {
		return nil, fmt.Errorf("image %s is not in the local docker daemon", ref)
	}
	return labels, nil
}

func (f *fakeDocker) Login(host, _, _ string) error {
	f.logins = append(f.logins, host)
	return nil
}

func (f *fakeDocker) Push(ref string) error {
	f.pushed = append(f.pushed, ref)
	return nil
}

func baseConfig(repo *testgit.Repo, mode types.Mode) types.Config {
	return types.Config{
		Mode:             mode,
		CommitSHA:        repo.Commit,
		GitRemote:        repo.Remote,
		Branch:           "main",
		PipelineID:       "1",
		MajorMinorSource: "cargo",
		MajorMinorFile:   filepath.Join(repo.Work, "Cargo.toml"),
		OutputDir:        repo.Work,
		Images:           []string{image},
	}
}

func setLine(t *testing.T, repo *testgit.Repo, version string) {
	t.Helper()
	body := fmt.Sprintf("[package]\nname = \"app\"\nversion = %q\n", version)
	if err := os.WriteFile(filepath.Join(repo.Work, "Cargo.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTag(t *testing.T, dir string) string {
	t.Helper()
	tag, err := artifacts.ReadReleaseTag(dir)
	if err != nil {
		t.Fatal(err)
	}
	return tag
}

// build runs mode version for one pipeline and returns the claimed version.
func build(t *testing.T, repo *testgit.Repo, pipeline string) string {
	t.Helper()
	cfg := baseConfig(repo, types.ModeVersion)
	cfg.PipelineID = pipeline
	if err := runVersion(cfg); err != nil {
		t.Fatalf("version (pipeline %s): %v", pipeline, err)
	}
	return readTag(t, repo.Work)
}

func deployCfg(repo *testgit.Repo, mode types.Mode, target string) types.Config {
	cfg := baseConfig(repo, mode)
	cfg.Event = "deployment"
	cfg.DeployTarget = target
	return cfg
}

func TestVersionClaimsSequentialNumbers(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "1.69.0")

	if got := build(t, repo, "10"); got != "1.69.0" {
		t.Fatalf("first build: got %s", got)
	}
	if got := build(t, repo, "11"); got != "1.69.1" {
		t.Fatalf("second build: got %s", got)
	}
	refs := repo.RemoteRefs()
	for _, ref := range []string{"refs/builds/1.69.0", "refs/builds/1.69.1", "refs/build-pipelines/10/1.69.0", "refs/build-pipelines/11/1.69.1"} {
		if _, ok := refs[ref]; !ok {
			t.Errorf("missing %s on the remote", ref)
		}
	}
	for ref := range refs {
		if strings.HasPrefix(ref, "refs/tags/") {
			t.Errorf("a build must not create a git tag, found %s", ref)
		}
	}
}

func TestVersionSkipsNumbersAlreadyTagged(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "1.69.0")
	repo.AnnotatedTag("1.69.41", repo.Commit) // a tag from the old pipeline-number scheme

	if got := build(t, repo, "5"); got != "1.69.42" {
		t.Fatalf("got %s, want 1.69.42", got)
	}
}

func TestVersionReusesTheNumberItsPipelineClaimed(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "0.3.0")

	web := build(t, repo, "77")
	android := build(t, repo, "77")
	if web != android {
		t.Fatalf("two workflows of one pipeline got %s and %s", web, android)
	}
	if got := build(t, repo, "78"); got != "0.3.1" {
		t.Fatalf("next pipeline: got %s", got)
	}
}

// Racing builds of one commit are the case a bare-commit ref would get wrong:
// each must still end up with its own number.
func TestVersionConcurrentBuildsNeverShareANumber(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "2.0.0")

	const builds = 6
	results := make([]string, builds)
	errs := make([]error, builds)
	var wg sync.WaitGroup
	for i := range builds {
		dir := t.TempDir()
		repo.Git(dir, "clone", "-q", repo.Remote, "w")
		work := filepath.Join(dir, "w")
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := baseConfig(repo, types.ModeVersion)
			cfg.PipelineID = fmt.Sprint(100 + i)
			cfg.OutputDir = work
			if errs[i] = runVersion(cfg); errs[i] == nil {
				results[i], errs[i] = artifacts.ReadReleaseTag(work)
			}
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("build %d: %v", i, err)
		}
	}
	sort.Strings(results)
	want := []string{"2.0.0", "2.0.1", "2.0.2", "2.0.3", "2.0.4", "2.0.5"}
	if strings.Join(results, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v, want %v", results, want)
	}
}

func TestVersionRefusesDeploymentEvents(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "1.0.0")
	cfg := baseConfig(repo, types.ModeVersion)
	cfg.Event = "deployment"
	if err := runVersion(cfg); err == nil {
		t.Fatal("version ran in a deployment pipeline")
	}
}

func TestVersionDryRunClaimsNothing(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "1.0.0")
	cfg := baseConfig(repo, types.ModeVersion)
	cfg.DryRun = true
	if err := runVersion(cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.RemoteRefs()["refs/builds/1.0.0"]; ok {
		t.Fatal("dry run claimed a build ref")
	}
}

func TestResolveDeploysNewestPublishedBuild(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "1.69.0")
	reg := newFakeRegistry()
	green := build(t, repo, "1") // 1.69.0, published
	reg.add(green, repo.Commit)
	build(t, repo, "2") // 1.69.1, failed: never published

	if err := runResolve(deployCfg(repo, types.ModeResolve, "dev"), reg); err != nil {
		t.Fatal(err)
	}
	if got := readTag(t, repo.Work); got != green {
		t.Fatalf("resolved %s, want the newest published build %s", got, green)
	}
}

// Dev and prod in either order: whichever deploys first tags the build, and the
// other finds that tag even when a newer green build of the commit exists.
func TestResolveFollowsTheTagInEitherOrder(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "1.0.0")
	reg := newFakeRegistry()
	first := build(t, repo, "1")
	reg.add(first, repo.Commit)

	prod := deployCfg(repo, types.ModeResolve, "prod")
	if err := runResolve(prod, reg); err != nil {
		t.Fatal(err)
	}
	if err := runTag(deployCfg(repo, types.ModeTag, "prod")); err != nil {
		t.Fatal(err)
	}

	newer := build(t, repo, "2") // a re-run of the build pipeline
	reg.add(newer, repo.Commit)

	if err := runResolve(deployCfg(repo, types.ModeResolve, "dev"), reg); err != nil {
		t.Fatal(err)
	}
	if got := readTag(t, repo.Work); got != first {
		t.Fatalf("dev after prod resolved %s, want prod's build %s", got, first)
	}
}

// The default prod rule follows the repo's trunk, whether main or master.
func TestResolveDefaultProdFollowsTheDefaultBranch(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "1.0.0")
	repo.Git(repo.Work, "push", "-q", repo.Remote, "main:master")
	repo.Git(repo.Remote, "symbolic-ref", "HEAD", "refs/heads/master")
	reg := newFakeRegistry()
	reg.add(build(t, repo, "1"), repo.Commit)

	cfg := deployCfg(repo, types.ModeResolve, "prod")
	cfg.Branch = "main"
	if err := runResolve(cfg, reg); err == nil || !strings.Contains(err.Error(), "set environments to change") {
		t.Fatalf("main on a master-trunk repo: got %v", err)
	}
	cfg.Branch = "master"
	if err := runResolve(cfg, reg); err != nil {
		t.Fatalf("master on a master-trunk repo: %v", err)
	}

	// Woodpecker names the default branch itself; no remote lookup needed.
	cfg.DefaultBranch = "main"
	cfg.Branch = "main"
	if err := runResolve(cfg, reg); err != nil {
		t.Fatalf("CI_REPO_DEFAULT_BRANCH=main: %v", err)
	}
}

// If the git server omits `^{}` lines, resolve must say so rather than report
// that the commit has no build.
func TestRequirePeeledBuilds(t *testing.T) {
	unpeeled := []types.Ref{{Name: "refs/tags/1.0.0", SHA: "c1"}, {Name: "refs/builds/1.0.1", SHA: "tagobj"}}
	if err := requirePeeledBuilds(unpeeled); err == nil || !strings.Contains(err.Error(), "without peeled") {
		t.Fatalf("got %v", err)
	}
	mixed := append(unpeeled, types.Ref{Name: "refs/builds/1.0.2", SHA: "tagobj2", Peeled: "c2"})
	if err := requirePeeledBuilds(mixed); err != nil {
		t.Fatalf("one peeled build ref proves the server peels: %v", err)
	}
	if err := requirePeeledBuilds(nil); err != nil {
		t.Fatalf("no build refs at all: %v", err)
	}
}

func TestResolveRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, repo *testgit.Repo, reg *fakeRegistry, cfg *types.Config)
		want  string
	}{
		{"unknown target", func(_ *testing.T, _ *testgit.Repo, _ *fakeRegistry, cfg *types.Config) {
			cfg.DeployTarget = "staging"
		}, "is not in the default environments"},
		{"branch not allowed", func(_ *testing.T, _ *testgit.Repo, _ *fakeRegistry, cfg *types.Config) {
			cfg.DeployTarget = "prod"
			cfg.Branch = "feat/x"
		}, "may not deploy to prod"},
		{"not a deployment", func(_ *testing.T, _ *testgit.Repo, _ *fakeRegistry, cfg *types.Config) {
			cfg.Event = "push"
		}, "deployment pipelines"},
		{"no build", func(*testing.T, *testgit.Repo, *fakeRegistry, *types.Config) {}, "push a fix or re-run the build"},
		{"build never published", func(t *testing.T, repo *testgit.Repo, _ *fakeRegistry, _ *types.Config) {
			build(t, repo, "1")
		}, "no published build"},
		{"tagged build pruned", func(_ *testing.T, repo *testgit.Repo, _ *fakeRegistry, _ *types.Config) {
			repo.AnnotatedTag("1.0.3", repo.Commit)
		}, "no longer in the registry"},
		{"two tags", func(_ *testing.T, repo *testgit.Repo, _ *fakeRegistry, _ *types.Config) {
			repo.AnnotatedTag("1.0.3", repo.Commit)
			repo.AnnotatedTag("1.0.4", repo.Commit)
		}, "more than one release tag"},
		{"label for another commit", func(t *testing.T, repo *testgit.Repo, reg *fakeRegistry, _ *types.Config) {
			reg.add(build(t, repo, "1"), "0000000000000000000000000000000000000000")
		}, "want commit"},
		{"registry cannot answer", func(t *testing.T, repo *testgit.Repo, reg *fakeRegistry, _ *types.Config) {
			build(t, repo, "1")
			reg.err = errors.New("registry cannot answer: 503")
		}, "cannot answer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := testgit.New(t)
			setLine(t, repo, "1.0.0")
			reg := newFakeRegistry()
			cfg := deployCfg(repo, types.ModeResolve, "dev")
			tc.setup(t, repo, reg, &cfg)
			os.Remove(filepath.Join(repo.Work, artifacts.ReleaseTagFile))

			err := runResolve(cfg, reg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
			if _, statErr := os.Stat(filepath.Join(repo.Work, artifacts.ReleaseTagFile)); statErr == nil {
				t.Fatal("a refused resolve must not write .release-tag")
			}
		})
	}
}

func TestTagPushesOnceAndIsIdempotent(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "1.2.0")
	build(t, repo, "1") // 1.2.0, now in .release-tag
	cfg := deployCfg(repo, types.ModeTag, "dev")
	if err := runTag(cfg); err != nil {
		t.Fatal(err)
	}
	obj := repo.RemoteRefs()["refs/tags/1.2.0"]
	if obj == "" {
		t.Fatal("tag not pushed")
	}
	if got := repo.Git(repo.Remote, "cat-file", "-t", obj); got != "tag" {
		t.Fatalf("want an annotated tag, got a %s", got)
	}
	if err := runTag(cfg); err != nil {
		t.Fatalf("redeploy: %v", err)
	}
	if got := repo.RemoteRefs()["refs/tags/1.2.0"]; got != obj {
		t.Fatal("redeploy moved the tag")
	}
}

func TestTagRefusesASecondVersionOrAMovedTag(t *testing.T) {
	repo := testgit.New(t)
	repo.AnnotatedTag("1.2.3", repo.Commit)
	cfg := deployCfg(repo, types.ModeTag, "dev")

	if err := artifacts.WriteReleaseTag(repo.Work, "1.2.4", ""); err != nil {
		t.Fatal(err)
	}
	if err := runTag(cfg); err == nil || !strings.Contains(err.Error(), "already has release tag") {
		t.Fatalf("second tag on a commit: got %v", err)
	}

	cfg.CommitSHA = repo.NewCommit("second")
	if err := artifacts.WriteReleaseTag(repo.Work, "1.2.3", ""); err != nil {
		t.Fatal(err)
	}
	if err := runTag(cfg); err == nil || !strings.Contains(err.Error(), "never move") {
		t.Fatalf("moving a tag: got %v", err)
	}
}

// A stale or hand-written .release-tag naming another commit's build must not
// pin this commit to it.
func TestTagRefusesAnotherCommitsBuild(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "1.2.0")
	version := build(t, repo, "1") // a build of the first commit

	cfg := deployCfg(repo, types.ModeTag, "dev")
	cfg.CommitSHA = repo.NewCommit("second")
	if err := runTag(cfg); err == nil || !strings.Contains(err.Error(), "is not a build of commit") {
		t.Fatalf("got %v", err)
	}
	if _, ok := repo.RemoteRefs()["refs/tags/"+version]; ok {
		t.Fatal("tagged the second commit with the first commit's build")
	}
}

func TestPublishPushesAfterEveryCheck(t *testing.T) {
	repo := testgit.New(t)
	if err := artifacts.WriteReleaseTag(repo.Work, "1.2.3", ""); err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{registry.LabelVersion: "1.2.3", registry.LabelRevision: repo.Commit}
	cli := image + "-cli"
	dk := &fakeDocker{local: map[string]map[string]string{image + ":1.2.3": labels, cli + ":1.2.3": labels}}
	cfg := baseConfig(repo, types.ModePublish)
	cfg.Images = []string{image, cli}
	cfg.RegistryUsername, cfg.RegistryPassword = "ci", "pw"

	if err := runPublish(cfg, newFakeRegistry(), dk); err != nil {
		t.Fatal(err)
	}
	if strings.Join(dk.pushed, " ") != image+":1.2.3 "+cli+":1.2.3" {
		t.Fatalf("pushed %v", dk.pushed)
	}
	if len(dk.logins) != 1 || dk.logins[0] != "registry.example" {
		t.Fatalf("want one login to registry.example, got %v", dk.logins)
	}
}

func TestPublishRefusesBeforePushingAnything(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		reg    func(*fakeRegistry, string)
		want   string
	}{
		{"wrong version label", map[string]string{registry.LabelVersion: "1.2.2"}, nil, registry.LabelVersion},
		{"already published", nil, func(r *fakeRegistry, commit string) { r.add("1.2.3", commit) }, "already published"},
		{"registry cannot answer", nil, func(r *fakeRegistry, _ string) { r.err = errors.New("registry cannot answer: timeout") }, "cannot answer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := testgit.New(t)
			if err := artifacts.WriteReleaseTag(repo.Work, "1.2.3", ""); err != nil {
				t.Fatal(err)
			}
			labels := tc.labels
			if labels == nil {
				labels = map[string]string{registry.LabelVersion: "1.2.3", registry.LabelRevision: repo.Commit}
			}
			dk := &fakeDocker{local: map[string]map[string]string{image + ":1.2.3": labels}}
			reg := newFakeRegistry()
			if tc.reg != nil {
				tc.reg(reg, repo.Commit)
			}
			err := runPublish(baseConfig(repo, types.ModePublish), reg, dk)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
			if len(dk.pushed) != 0 {
				t.Fatalf("pushed %v despite the refusal", dk.pushed)
			}
		})
	}
}

// The full path from push to deploy, as a donor pipeline runs it.
func TestBuildPublishResolveTag(t *testing.T) {
	repo := testgit.New(t)
	setLine(t, repo, "0.5.0")
	reg := newFakeRegistry()

	version := build(t, repo, "9")
	labels := map[string]string{registry.LabelVersion: version, registry.LabelRevision: repo.Commit}
	dk := &fakeDocker{local: map[string]map[string]string{image + ":" + version: labels}}
	if err := runPublish(baseConfig(repo, types.ModePublish), reg, dk); err != nil {
		t.Fatal(err)
	}
	reg.add(version, repo.Commit) // what the push put in the registry

	os.Remove(filepath.Join(repo.Work, artifacts.ReleaseTagFile))
	if err := runResolve(deployCfg(repo, types.ModeResolve, "prod"), reg); err != nil {
		t.Fatal(err)
	}
	if err := runTag(deployCfg(repo, types.ModeTag, "prod")); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.RemoteRefs()["refs/tags/"+version]; !ok {
		t.Fatalf("tag %s not pushed", version)
	}
}
