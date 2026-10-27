package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestFillFromBuildInfoUsesModuleVersionAndRevision(t *testing.T) {
	info := Info{Version: "dev", Commit: "unknown", BuiltAt: "unknown"}
	fillFromBuildInfo(&info, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "cee46a218808d19ff68771d2037b9e8852a82623"},
			{Key: "vcs.time", Value: "2026-09-04T23:41:41Z"},
		},
	})
	want := Info{Version: "0.1.0", Commit: "cee46a218808d19ff68771d2037b9e8852a82623", BuiltAt: "2026-09-04T23:41:41Z"}
	if info != want {
		t.Fatalf("got %+v, want %+v", info, want)
	}
}

func TestFillFromBuildInfoKeepsReleaseStamp(t *testing.T) {
	info := Info{Version: "0.1.0", Commit: "abc1234", BuiltAt: "unknown"}
	fillFromBuildInfo(&info, &debug.BuildInfo{
		Main:     debug.Module{Version: "v9.9.9"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "ffff"}},
	})
	if info.Version != "0.1.0" || info.Commit != "abc1234" {
		t.Fatalf("release stamp was replaced: %+v", info)
	}
}

func TestFillFromBuildInfoIgnoresDevelopmentBuild(t *testing.T) {
	info := Info{Version: "dev", Commit: "unknown", BuiltAt: "unknown"}
	fillFromBuildInfo(&info, &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}})
	if info.Version != "dev" {
		t.Fatalf("got version %q, want dev", info.Version)
	}
}
