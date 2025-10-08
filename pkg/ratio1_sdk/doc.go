// Package ratio1_sdk provides helpers to bootstrap CStore and R1FS clients
// based on environment variables conventionally exposed inside Ratio1 Edge
// Nodes. The behaviour is documented in detail in README.md and mirrors the
// runtime contract described for R1_RUNTIME_MODE, EE_CHAINSTORE_API_URL, and
// EE_R1FS_API_URL. When the upstream REST services are unavailable, the helpers
// produce in-memory mocks that remain API compatible with the HTTP clients.
package ratio1_sdk
