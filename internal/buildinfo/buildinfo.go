// Package buildinfo reports which commit this binary was built from, as the Go
// toolchain stamped it into the binary. It is the one place inber reads that
// stamp, so the health route, the -version flag and the prompt's source line
// cannot disagree about it.
package buildinfo

import (
	"runtime/debug"
	"sync"
)

// Build is the version-control stamp of the running binary. Revision is empty
// when the binary carries no stamp (`go run`, -buildvcs=false, or a build from a
// directory with no .git); callers must treat empty as "unknown", not guess.
type Build struct {
	Revision string `json:"revision"`
	Time     string `json:"time,omitempty"`
	Modified bool   `json:"modified"`
}

var (
	readOnce sync.Once
	current  Build
)

// Current returns the stamp of the running binary. It reads the stamp once.
func Current() Build {
	readOnce.Do(func() {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		current = FromSettings(info.Settings)
	})
	return current
}

// FromSettings picks the vcs.* keys out of a binary's build settings.
func FromSettings(settings []debug.BuildSetting) Build {
	var build Build
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			build.Revision = setting.Value
		case "vcs.time":
			build.Time = setting.Value
		case "vcs.modified":
			build.Modified = setting.Value == "true"
		}
	}
	return build
}
