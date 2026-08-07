package session

// The four token estimators that used to live here — estimateTokens,
// estimateToolTokens, estimateSystemTokens and estimateMessageTokens — are
// gone. They were a second, and worse, copy of the estimator in package
// conversation, and the prompts/*.md breakdown they rendered is the one place
// a human goes to find out where a turn's context went.
//
// The message walk counted a text block's characters and charged a flat 50
// tokens for a tool_use or a tool_result. A tool result is the largest thing
// in an agentic conversation, so the breakdown under-reported exactly the
// sessions that most need reading. Measured 2026-08-07 over the 92 persisted
// transcripts under ~/.inber/server/sessions: the honest estimator is a
// median of 1.35x the old walk, and up to 6.89x — one sub-agent transcript
// was reported as 7,895 tokens against an honest 54,415, hiding 46,500. On a
// single 20KB read_files result the old walk answers 117 against 7,066.
//
// This is the same bug conversation.estimateMessageTokens' doc comment
// describes having already been fixed once, surviving in a copy nothing
// walked.
//
// Their replacements are conversation.EstimateMessageTokens,
// EstimateSystemTokens / EstimateSystemBlockTokens and EstimateToolsTokens /
// EstimateToolTokens, all of which price a value by marshalling it the way it
// will be sent, and conversation.EstimateRequestTokens for the whole request.
// Do not reintroduce a local copy: session already imports conversation, so
// the "kept local so the session package does not depend on the memory layer"
// reason the old file gave was not true when it was written.
