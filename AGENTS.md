# Random Chess Backend (Codex)

Goal: anonymous user makes 1 legal move in assigned ongoing game; server authoritative.

Contracts:
- openapi.yaml lives in random-chess-contract (source of truth).
- Do not invent fields. If needed: change contract repo first.

Rules:
- Never mention AI/Claude/Codex in code, docs, commits, PR text.
- Do not add new prompt/AI files beyond existing repo set.
- No domain logic in HTTP handlers.
- Validate move legality server-side; never trust client state/FEN.
- Concurrency-safe submit (expected_version/state_version).
- Small diffs; no refactors unless required.

Process:
ARCHITECT (if unclear) -> PLAN (<=10 bullets) -> IMPLEMENT -> REVIEW -> TEST