package version

// Version is the release line (MAJOR.MINOR.PATCH). The patch digit is a
// placeholder; deploy tooling allocates the real patch from remote tags.
const Version = "0.2.0"

// ReleaseTag is set at image build time via -ldflags for the deployed semver.
var ReleaseTag = Version
