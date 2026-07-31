package conversation

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// StagedConversation manages a conversation in two zones:
//   - Frozen zone (index 0..FrozenIdx-1): never mutated, always cache-hit
//   - Staging zone (index FrozenIdx..len-1): freely mutated, uncached
//
// Every FlushInterval turns, staging is frozen (FrozenIdx advances).
// Between flushes, dedup and pruning only touch the staging zone.
type StagedConversation struct {
	FrozenIdx     int // messages before this index are frozen (cache-stable)
	FlushInterval int // how many turns between freezes
	TurnsSinceFlush int
}

// NewStagedConversation creates a staged conversation manager.
func NewStagedConversation(flushInterval int) *StagedConversation {
	if flushInterval <= 0 {
		flushInterval = 5
	}
	return &StagedConversation{
		FrozenIdx:     0,
		FlushInterval: flushInterval,
	}
}

// ShouldFlush returns true if it's time to freeze the staging zone.
func (sc *StagedConversation) ShouldFlush() bool {
	return sc.TurnsSinceFlush >= sc.FlushInterval
}

// Flush freezes the staging zone: advances FrozenIdx to cover all current messages.
func (sc *StagedConversation) Flush(messageCount int) {
	sc.FrozenIdx = messageCount
	sc.TurnsSinceFlush = 0
}

// Tick increments the turn counter.
func (sc *StagedConversation) Tick() {
	sc.TurnsSinceFlush++
}

// ShiftAfterHeadDrop moves the frozen boundary to follow messages dropped off the
// head of the conversation.
//
// FrozenIdx is an index into the message slice and nothing else in this type is,
// so anything that shortens the slice from the front moves every message the
// boundary was placed against. Left unshifted the boundary points `dropped`
// positions too far in, and the "staging zone" it then names starts inside the
// frozen zone — which ManageStaging is free to dedup and prune, the one thing the
// frozen zone exists to prevent.
//
// A boundary that falls off the front collapses to zero, the "nothing is frozen
// yet" value it starts at.
func (sc *StagedConversation) ShiftAfterHeadDrop(dropped int) {
	sc.FrozenIdx -= dropped
	if sc.FrozenIdx < 0 {
		sc.FrozenIdx = 0
	}
}

// FreezePoint returns the index up to which a transcript may be frozen.
//
// Everything is frozen except a trailing user message. The next turn appends
// its input by merging into a trailing user message rather than adding a
// second one — breaking alternation would be rejected by the API — so that
// message is not yet final and freezing it would mutate the frozen zone.
func FreezePoint(messages []anthropic.MessageParam) int {
	n := len(messages)
	if n > 0 && messages[n-1].Role == anthropic.MessageParamRoleUser {
		return n - 1
	}
	return n
}

// StagingSlice returns only the staging zone messages for mutation.
// The returned slice shares the underlying array — mutations affect the original.
func (sc *StagedConversation) StagingSlice(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if sc.FrozenIdx >= len(messages) {
		return nil
	}
	return messages[sc.FrozenIdx:]
}

// ManageStaging applies dedup and pruning ONLY to the staging zone.
// The frozen zone is never touched, preserving cache stability.
func ManageStaging(
	messages []anthropic.MessageParam,
	frozenIdx int,
	cfg ManagementConfig,
) (deduped int) {
	if frozenIdx >= len(messages) {
		return 0
	}

	staging := messages[frozenIdx:]

	// Dedup file references within staging (free — no cache cost to mutate)
	deduped = DeduplicateFileRefs(staging)

	// Also dedup staging against frozen: if staging reads a file that was
	// read in frozen zone, mark the frozen one as superseded... wait, NO.
	// We can't mutate frozen. Instead, check if staging reads a file that
	// was already read in frozen, and mark the STAGING one with context.
	// Actually: the model should use the LATEST read. If frozen has an old
	// read and staging has a new read, the old one is stale but we can't
	// touch it. The model will see both — old (stale) and new (current).
	// On the NEXT flush, when old moves further into frozen history, it'll
	// naturally age out when eventually the entire conversation gets
	// compacted. For now, this is acceptable.

	// Age-based pruning on staging zone
	// Staging messages are recent (within FlushInterval turns), so most
	// won't hit the prune thresholds. But if FlushInterval is large or
	// emergency management fires, some might qualify.
	stagingAges := computeAges(staging)
	for i := range staging {
		if staging[i].Role != anthropic.MessageParamRoleUser {
			continue
		}
		age := stagingAges[i]
		for j := range staging[i].Content {
			block := staging[i].Content[j]
			if block.OfToolResult != nil {
				pruned, wasPruned := pruneToolResult(block, age, cfg)
				if wasPruned {
					staging[i].Content[j] = pruned
				}
			}
		}
	}

	return deduped
}

// CrossZoneDedup checks if the staging zone reads files that were already read
// in the frozen zone, and replaces the OLDER (frozen) reference info in a
// summary note appended to the volatile context. Does NOT mutate frozen messages.
// Returns file paths that were superseded.
func CrossZoneDedup(frozen, staging []anthropic.MessageParam) []string {
	// Collect file paths read/written in frozen zone
	frozenPaths := make(map[string]bool)
	for _, msg := range frozen {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			paths, ok := toolUseFilePaths(block)
			if !ok {
				continue
			}
			for _, path := range paths {
				frozenPaths[path] = true
			}
		}
	}

	// Find which staging operations reference the same files
	var superseded []string
	for _, msg := range staging {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			paths, ok := toolUseFilePaths(block)
			if !ok {
				continue
			}
			for _, path := range paths {
				if frozenPaths[path] {
					superseded = append(superseded, path)
				}
			}
		}
	}

	return superseded
}

// computeAges returns per-message age (turns from end) for a message slice.
func computeAges(messages []anthropic.MessageParam) []int {
	ages := make([]int, len(messages))
	turn := 0
	for i := len(messages) - 1; i >= 0; i-- {
		ages[i] = turn
		if messages[i].Role == anthropic.MessageParamRoleUser {
			turn++
		}
	}
	return ages
}
