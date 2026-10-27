package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

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

// Current returns the build metadata injected by the release pipeline. Builds without it, such as
// `go install …@v0.1.0` or a source checkout, fall back to what the Go toolchain recorded.
func Current() Info {
	info := Info{
		Version:   version,
		Commit:    commit,
		BuiltAt:   builtAt,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
	if recorded, ok := debug.ReadBuildInfo(); ok {
		fillFromBuildInfo(&info, recorded)
	}
	return info
}

// fillFromBuildInfo completes only the fields the release pipeline left unset. A module version
// reads like the release pipeline's own, without its leading "v".
func fillFromBuildInfo(info *Info, recorded *debug.BuildInfo) {
	if info.Version == "dev" {
		if v := recorded.Main.Version; v != "" && v != "(devel)" {
			info.Version = strings.TrimPrefix(v, "v")
		}
	}
	for _, setting := range recorded.Settings {
		switch setting.Key {
		case "vcs.revision":
			if info.Commit == "unknown" {
				info.Commit = setting.Value
			}
		case "vcs.time":
			if info.BuiltAt == "unknown" {
				info.BuiltAt = setting.Value
			}
		}
	}
}
