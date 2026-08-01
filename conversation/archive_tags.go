package conversation

import "github.com/kayushkin/inber/memory"

// Both writers in this package save a memory in order to take content *out* of a
// conversation: SummarizeConversation archives the turns a summary replaces, and
// StashLargeContent lifts a large block out of a message and leaves a pointer.
// Neither wants its content offered back to the next prompt automatically — that
// would undo the saving the write was made for — so both tag their memory with
// one of memory.TagsExcludedFromAutomaticContext.
//
// The tag sets are built here rather than spelled inline at each Save so a test
// can assert the property that actually matters: that what a writer attaches
// intersects what the reader excludes. Spelled inline, the compaction archive
// carried `conversation`/`history` and the stash carried `large-input`/`stashed`
// while the reader excluded `session-summary`, and nothing noticed for months.

// ConversationArchiveTags are the tags on the verbatim transcript that
// SummarizeConversation stores when it replaces older turns with a summary.
//
// The session tag keeps the archive attributable; it does not scope any read.
// BuildContext has no notion of a session, which is why the archive has to be
// excluded by tag rather than left to be filtered by one.
func ConversationArchiveTags(sessionID string) []string {
	return []string{memory.TagConversationArchive, "conversation", "history", "session:" + sessionID}
}

// StashedContentTags are the tags on a large block that StashLargeContent moved
// out of a message and into memory.
func StashedContentTags(sessionID string, contentType ContentType) []string {
	return []string{"large-input", memory.TagStashedContent, sessionID, string(contentType)}
}
