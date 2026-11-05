// Package provider is how flores talks to a language model without depending
// on any vendor (ADR 0002).
//
// It defines our own types (Message, Request, Event, Error) and the single
// Provider interface. Each wire protocol lives in its own subpackage
// (provider/anthropic, …) and translates between these types and the bytes
// on the wire.
//
// What this package must NOT do: know about tools' behaviour, the agent loop,
// retries policy or output formats. No vendor or protocol type ever leaves a
// subpackage.
package provider
