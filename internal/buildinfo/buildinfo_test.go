package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestFromSettingsReadsTheVersionControlStamp(t *testing.T) {
	build := FromSettings([]debug.BuildSetting{
		{Key: "-compiler", Value: "gc"},
		{Key: "vcs.revision", Value: "0eb9b66857c8d587696488ed4967b4e93b621bcf"},
		{Key: "vcs.time", Value: "2026-11-06T16:46:42Z"},
		{Key: "vcs.modified", Value: "true"},
	})
	want := Build{Revision: "0eb9b66857c8d587696488ed4967b4e93b621bcf", Time: "2026-11-06T16:46:42Z", Modified: true}
	if build != want {
		t.Fatalf("FromSettings = %+v, want %+v", build, want)
	}
}

func TestFromSettingsLeavesAnUnstampedBuildEmpty(t *testing.T) {
	build := FromSettings([]debug.BuildSetting{{Key: "-compiler", Value: "gc"}})
	if build != (Build{}) {
		t.Fatalf("FromSettings on an unstamped build = %+v, want the zero Build", build)
	}
}

func TestFromSettingsReadsOnlyTheLiteralTrueAsModified(t *testing.T) {
	build := FromSettings([]debug.BuildSetting{{Key: "vcs.modified", Value: "false"}})
	if build.Modified {
		t.Fatal(`vcs.modified="false" read as modified`)
	}
}
