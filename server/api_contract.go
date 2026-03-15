package server

// API Contract — HTTP interfaces between inber server and its clients.
//
// Clients: bus-agent (HTTPBackend), dashboard, future adapters.
// This file documents the wire format. Changes here must be
// coordinated with bus-agent's HTTPBackend and si's bus feed.
//
// POST /api/run
//   Request:  RunRequest  → defined in server.go
//   Response: RunResponse → defined in server.go
//   Stream:   Accept: text/event-stream → SSE StreamEvents
//
// RunRequest fields:
//   agent       string  (optional, routes to default agent)
//   message     string  (required)
//   session_key string  (optional, auto-generated if empty)
//   channel     string  (optional, for context)
//   author      string  (optional, prefixed to message)
//
// RunResponse fields:
//   text        string
//   session_key string
//   tokens      TokenUsage {input, output, cache_read, cache_write, cost}
//   duration_ms int
//
// StreamEvent fields:
//   kind  string  ("delta", "thinking", "tool_call", "tool_result", "done")
//   text  string
//   tool  string
//   data  any     (on "done": {tokens, duration_ms})
//
// Bus message format (inbound/outbound topics):
//   {id, topic, payload, timestamp}
//   payload is JSON-encoded RunRequest (inbound) or RunResponse (outbound)
