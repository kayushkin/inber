package engine

import "github.com/kayushkin/inber/agent"

// mergeExtraTools overlays server-injected tools onto the built-in set: an
// extra tool whose name already appears replaces that entry, and one with a new
// name is appended.
//
// It replaces the FIRST match only. If the base set already holds the same name
// twice, the second copy survives the merge and both definitions still go on
// the wire, where dispatch resolves the name to whichever was registered last.
// That is a real defect, tracked as noteboard todo
// e2d0b07b-5034-4f7d-b97f-2a534141dfc1; it needs a policy decision (reject a
// duplicate at registration, or keep the first and warn), so this function
// preserves the existing behaviour rather than guessing at it.
//
// This loop used to exist twice — engine.initTools and buildEngineWithTiming —
// so a fix to one left the other wrong.
func mergeExtraTools(base []agent.Tool, extras []agent.Tool) []agent.Tool {
	for _, extra := range extras {
		replaced := false
		for i, existing := range base {
			if existing.Name == extra.Name {
				base[i] = extra
				replaced = true
				break
			}
		}
		if !replaced {
			base = append(base, extra)
		}
	}
	return base
}
