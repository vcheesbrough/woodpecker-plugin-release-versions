package gitremote

import (
	"testing"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/testgit"
)

func TestParseLsRemoteFoldsPeeledLines(t *testing.T) {
	stdout := "abc111\trefs/tags/1.2.0\ndef222\trefs/tags/1.2.0^{}\nghi333\trefs/tags/1.2.1\n"
	refs := ParseLsRemote(stdout)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %#v", refs)
	}
	if refs[0].Name != "refs/tags/1.2.0" || refs[0].SHA != "abc111" || refs[0].Commit() != "def222" {
		t.Fatalf("annotated tag not peeled: %#v", refs[0])
	}
	if refs[1].Commit() != "ghi333" {
		t.Fatalf("lightweight tag should name its commit directly: %#v", refs[1])
	}
}

// Build refs point at tag objects outside refs/tags. resolve finds a commit's
// builds only through the peeled line, so this drives real git, not a string.
func TestListRefsPeelsTagObjectsOutsideRefsTags(t *testing.T) {
	repo := testgit.New(t)
	remote := Remote{URL: repo.Remote}

	obj, err := remote.MakeTag(repo.Work, TagObject{Commit: repo.Commit, Name: "1.0.0", Message: "Build 1.0.0\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Push(repo.Work, Update{Object: obj, Ref: "refs/builds/1.0.0"}); err != nil {
		t.Fatal(err)
	}

	refs, err := remote.ListRefs("refs/builds/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].SHA != obj || refs[0].Commit() != repo.Commit {
		t.Fatalf("want refs/builds/1.0.0 -> %s peeled to %s, got %#v", obj, repo.Commit, refs)
	}
	if _, ok := repo.RemoteRefs()["refs/tags/1.0.0"]; ok {
		t.Fatal("MakeTag must not create a tag ref")
	}
}

func TestPushIsCreateOnly(t *testing.T) {
	repo := testgit.New(t)
	remote := Remote{URL: repo.Remote}

	first, err := remote.MakeTag(repo.Work, TagObject{Commit: repo.Commit, Name: "1.0.0", Message: "pipeline 1\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Push(repo.Work, Update{Object: first, Ref: "refs/builds/1.0.0"}); err != nil {
		t.Fatal(err)
	}

	// Same commit, different pipeline: a distinct object, so the push must be
	// refused rather than treated as a no-op.
	second, err := remote.MakeTag(repo.Work, TagObject{Commit: repo.Commit, Name: "1.0.0", Message: "pipeline 2\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Push(repo.Work, Update{Object: second, Ref: "refs/builds/1.0.0"}); err == nil {
		t.Fatal("second claim of an existing ref succeeded")
	}
	if got := repo.RemoteRefs()["refs/builds/1.0.0"]; got != first {
		t.Fatalf("ref moved to %s, want it to stay at %s", got, first)
	}
}

func TestPushIsAtomic(t *testing.T) {
	repo := testgit.New(t)
	remote := Remote{URL: repo.Remote}

	taken, _ := remote.MakeTag(repo.Work, TagObject{Commit: repo.Commit, Name: "1.0.0", Message: "a\n"})
	if err := remote.Push(repo.Work, Update{Object: taken, Ref: "refs/builds/1.0.0"}); err != nil {
		t.Fatal(err)
	}
	obj, _ := remote.MakeTag(repo.Work, TagObject{Commit: repo.Commit, Name: "1.0.0", Message: "b\n"})
	err := remote.Push(repo.Work,
		Update{Object: obj, Ref: "refs/build-pipelines/7/1.0.0"},
		Update{Object: obj, Ref: "refs/builds/1.0.0"},
	)
	if err == nil {
		t.Fatal("push with one taken ref succeeded")
	}
	if _, ok := repo.RemoteRefs()["refs/build-pipelines/7/1.0.0"]; ok {
		t.Fatal("pipeline ref was created although the build ref was refused")
	}
}

func TestRedactHidesToken(t *testing.T) {
	r := Remote{Token: "s3cret"}
	if got := r.redact("fatal: bad header s3cret"); got != "fatal: bad header ***" {
		t.Fatalf("got %q", got)
	}
}
