package buildinfo

import "runtime"

var (
	version = "dev"
	commit  = "unknown"
	builtAt = "unknown"
)

// Info describes the executable build without inspecting the network.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuiltAt   string `json:"built_at"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Current returns the build metadata injected by the release pipeline.
func Current() Info {
	return Info{
		Version:   version,
		Commit:    commit,
		BuiltAt:   builtAt,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}
