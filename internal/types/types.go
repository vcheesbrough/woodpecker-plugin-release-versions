package types

import "log"

type Mode string

const (
	ModeCompute  Mode = "compute"
	ModePushTag  Mode = "push-tag"
	ModeValidate Mode = "validate"
)

type Config struct {
	Mode             Mode
	CommitSHA        string
	Repo             string
	GitHubToken      string
	Branch           string
	MajorMinorSource string
	MajorMinorFile   string
	TagPrefix        string
	OutputDir        string
	DryRun           bool
	PipelineNumber   string
	Logger           *log.Logger
}

type RemoteTag struct {
	Name string
	SHA  string
}
