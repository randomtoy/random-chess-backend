# Random Chess Backend — Rules

Goal: anonymous users make exactly 1 legal move in an assigned ongoing game, then switch to next. Server is authoritative.

Contracts:
- API contract is in random-chess-contract/openapi.yaml (source of truth).
- Backend must not invent fields beyond contract. If needed -> update contract repo first.

Hard rules:
- Never mention AI/Claude/Codex in code, docs, commits, PR text.
- Do not add new AI/prompt files beyond those already in repo.
- No business logic in HTTP handlers.
- Validate chess legality server-side. Never trust client FEN/state.
- Small diffs; no refactors unless required.
- Concurrency-safe move submit (optimistic lock via state_version / expected_version).
- If contract must change -> PR to random-chess-contract first.

Architecture (MVP):
- transport/http: parse/validate input shape, call usecase, map errors -> HTTP
- application/usecases: orchestration (assign, submit move, fetch game)
- domain/chess: rules + transitions + state
- adapters: storage, rate limiter, clock, ids
- ports: interfaces for adapters

Abuse (MVP):
- rate limit by IP + optional X-Client-Token
- enforce “one move per game per fingerprint” (best-effort; handle missing token)

Workflow:
1) ARCHITECT (no code) if scope unclear
2) PLAN (<=10 bullets)
3) IMPLEMENT (minimal diffs)
4) TEST/VERIFY + short self-review