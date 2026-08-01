package memory

// This file is the single place that answers one question: which memories may be
// injected into a prompt without anybody asking for them.
//
// Three writers in this repository save a memory *in order to make a conversation
// smaller* — the session summary written at shutdown, the transcript archived when
// a conversation is compacted, and a large block moved out of a message by the
// stasher. Each of them removes content from the conversation and leaves the model
// a pointer to it (`memory_expand`, `memory_search`). None of them wants that
// content back in the next prompt: putting it back undoes the saving that was the
// reason for the write.
//
// That intent has to be spelled the same way at both ends. It was not:
// `BuildSystemPrompt` excluded `session-summary`, `repo-map` and
// `code-introspection`, of which only the first was ever written by anything —
// while the compaction archive tagged itself `conversation`/`history` and the
// stasher tagged itself `large-input`/`stashed`, so both walked straight past the
// exclusion that existed for exactly them. Measured on a store holding ten
// compaction archives and two stashed blocks, 12 of 13 assembled memories and 99%
// of the assembled tokens were archive fragments the reader never asked for, and
// they came from ten *other* sessions: the archives are session-scoped by tag and
// the reader has no session scope at all.
//
// So the tag names and the exclusion list live here together, and every writer and
// the reader take them from here. A writer that wants its memory kept out of the
// automatic context adds its tag to both — not to one.

const (
	// TagSessionSummary marks the digest of a finished session, written by
	// Engine.saveSessionSummary at shutdown.
	TagSessionSummary = "session-summary"

	// TagConversationArchive marks the verbatim transcript of the turns a
	// compaction replaced with a summary, written by
	// conversation.SummarizeConversation. Recoverable with memory_expand.
	TagConversationArchive = "conversation-archive"

	// TagStashedContent marks a large block lifted out of a message and replaced
	// in the conversation by a pointer, written by conversation.StashLargeContent.
	TagStashedContent = "stashed"
)

// TagsExcludedFromAutomaticContext are the tags whose memories exist so that a
// conversation could be made smaller. BuildContext must not offer them for
// automatic injection; they are reached by memory_search or memory_expand, by id,
// when something actually wants them.
//
// Every entry here is written by a named writer in this repository. An entry with
// no writer is not a safety margin — it is a line that reads as a policy and
// enforces nothing, which is how the previous version of this list came to hold
// two tags (`repo-map`, `code-introspection`) that no code has ever attached to a
// memory.
var TagsExcludedFromAutomaticContext = []string{
	TagSessionSummary,
	TagConversationArchive,
	TagStashedContent,
}

// AutomaticContextRequest is the read that assembles the memories in a system
// prompt — the only read that injects memories nobody asked for. Engine
// .BuildSystemPrompt makes it on every turn.
//
// It lives beside the tags it excludes, and not as a literal at the one call
// site, because its ExcludeTags is one half of a contract whose other half is
// the writers' tag sets. A literal buried in a 60-line method is where the two
// halves drifted apart, and a copy of it in a test is the same drift with a
// green tick on it.
func AutomaticContextRequest(messageTags []string, minImportance float64, tokenBudget int) BuildContextRequest {
	return BuildContextRequest{
		Tags:              messageTags,
		TokenBudget:       tokenBudget,
		MinImportance:     minImportance,
		IncludeAlwaysLoad: true,
		ExcludeTags:       TagsExcludedFromAutomaticContext,
		MaxChunkSize:      5000,
		TruncateThreshold: 1000, // only truncate if memory >1000 tokens AND preview is <50% of content
		TruncatePreview:   800,  // ~270 token preview — enough to understand what the memory is about
	}
}
