// Package interaction is the isolated SDK for the Agentic Interaction JSON-RPC
// bridge. It knows the native unix-socket transport, discovers the running app
// with system.describe, exposes method constants and typed request params for
// the current native surface, and keeps subscription connections alive for
// streaming RPC methods.
package interaction
