package types

type Mode string

const (
	ModeVersion Mode = "version"
	ModePublish Mode = "publish"
	ModeResolve Mode = "resolve"
	ModeTag     Mode = "tag"
)

// Environment is one deploy target's policy. Branches empty means any branch.
type Environment struct {
	Branches []string `json:"branches"`
}

type Config struct {
	Mode             Mode
	CommitSHA        string
	Repo             string
	GitHubToken      string
	GitRemote        string
	Branch           string
	Event            string
	DeployTarget     string
	PipelineID       string
	MajorMinorSource string
	MajorMinorFile   string
	TagPrefix        string
	OutputDir        string
	DryRun           bool
	Images           []string
	RegistryUsername string
	RegistryPassword string
	RegistryInsecure bool
	Environments     map[string]Environment
	GitHubOutput     string
}

// Ref is one line of `git ls-remote`. SHA is the object the ref points at; for
// an annotated tag object that is the tag, and Peeled is the commit inside it.
type Ref struct {
	Name   string
	SHA    string
	Peeled string
}

// Commit is the commit a ref ultimately names.
func (r Ref) Commit() string {
	if r.Peeled != "" {
		return r.Peeled
	}
	return r.SHA
}
