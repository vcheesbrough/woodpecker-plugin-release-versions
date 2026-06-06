package release

import "testing"

func TestIsDockerTagSafe(t *testing.T) {
	if IsDockerTagSafe("1.2.3+abc") {
		t.Fatal("expected + metadata to be rejected")
	}
	if !IsDockerTagSafe("1.2.3") {
		t.Fatal("expected plain semver to be accepted")
	}
}
