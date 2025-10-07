// Package cstore provides a lightweight client for the Ratio1 Chainstore (CStore)
// API. The HTTP surface mirrors the FastAPI plugin implemented in
// extensions/business/cstore/cstore_manager_api.py within the Ratio1 edge_node
// repository. The public Go API offers higher-level helpers (generic Get/Put
// operations, list pagination) while documenting gaps where the upstream REST
// implementation currently lacks features such as TTL or conditional headers.
package cstore
