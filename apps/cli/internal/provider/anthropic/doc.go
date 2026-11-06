// Package anthropic speaks the Anthropic Messages API (POST /v1/messages,
// streamed as server-sent events) with net/http and no SDK (ADR 0002).
//
// anthropic.go builds the request and maps HTTP errors; stream.go reads the
// SSE stream and turns it into provider events, assembling tool calls whose
// JSON arrives in fragments.
//
// What this package must NOT do: retry, decide what to do with tool calls, or
// let any wire type escape. Everything it returns is a provider type.
package anthropic
