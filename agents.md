# Overview

- Ratio1 Edge SDK for Go providing CStore (key/value) and R1FS (file storage) clients.
- CStore surface mirrors the Python manager with Set/Get/HSet/HGet/HGetAll/GetStatus only; JSON helpers and pagination are intentionally omitted.
- REST APIs sourced from Ratio1/edge_node:
  - https://github.com/Ratio1/edge_node/blob/main/extensions/business/cstore/cstore_manager_api.py
  - https://github.com/Ratio1/edge_node/blob/main/extensions/business/r1fs/r1fs_manager_api.py
- Clients initialise directly against live HTTP endpoints using `EE_CHAINSTORE_API_URL` and `EE_R1FS_API_URL`.

# Roles

- Repo Architect
  - Define package boundaries (`internal/httpx`, `pkg/cstore`, `pkg/r1fs`).
  - Maintain low dependency surface, consistent error types, Makefile, CI.
- SDK Author
  - Keep HTTP clients aligned with upstream Python APIs.
  - Expand tests and examples, ensure streaming paths remain efficient.
- DX Engineer
  - Maintain developer tooling, runnable examples, and environment diagnostics for live integrations.
  - Ensure setup instructions cover production and staging environments.
- Release Engineer
  - Own tagging strategy (v0.x), CI on tags, release notes coordination.
  - Track compatibility considerations in README.
- Test Engineer
  - Add unit tests for HTTP behaviours (including retries, context cancellation).
  - Maintain golden fixtures for JSON contracts.
- Docs Writer
  - Keep README, package docs, and API assumptions up to date.
  - Document TODOs referencing upstream sources.
- Security Auditor
  - Review headers, secrets handling, timeouts, retry policies.
  - Ensure repo remains free of accidental secrets.

# Prompts to reuse

```
Update endpoints from upstream
Read the latest r1fs_manager_api.py and cstore_manager_api.py in Ratio1/edge_node and regenerate endpoint paths, query params, headers, and response types in pkg/r1fs and pkg/cstore. Update tests and docs accordingly.
```

```
Improve retry/backoff
Refine internal/httpx retry to include jitter, cap retries by elapsed time, and propagate context cancellation correctly. Add tests for 429/503 and a non-retryable 4xx.
```

```
Expand R1FS streaming
Add multipart upload for large files if supported by the API; otherwise provide chunked upload fallback with content-range headers. Document limits.
```

```
Live endpoint validation
Build diagnostics that verify configured CStore and R1FS endpoints (auth, headers, latency) and report actionable errors.
```
