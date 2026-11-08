// Package openai speaks OpenAI Chat Completions (POST {base}/chat/completions,
// streamed as server-sent events) with net/http and no SDK (ADR 0002, 0005).
//
// The same protocol is spoken by most "OpenAI-compatible" servers: Ollama,
// LM Studio, vLLM, OpenRouter, Groq, Cloudflare's gateway… Only the base URL
// changes, and it already includes the version (".../v1").
//
// openai.go builds the request and maps HTTP errors; stream.go reads the SSE
// stream and turns it into provider events, assembling tool calls whose
// arguments arrive in fragments.
//
// What this package must NOT do: retry, decide what to do with tool calls, or
// let any wire type escape. Everything it returns is a provider type.
package openai
