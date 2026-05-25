# Architecture Decision Records (ADRs)

This directory holds the architectural decisions made for Lumen — *why* the code looks the way it does.

## How to read an ADR

Each ADR is a short markdown file capturing one decision:

- **Context** — the situation and forces at play
- **Decision** — what we chose
- **Consequences** — positives, negatives, neutral effects
- **Alternatives rejected** — what we considered and why we said no

ADRs are **append-only**. If a decision is later overturned, write a new ADR that *supersedes* the old one. Never edit the old one to "match" the new direction.

## Index

| # | Title | Status |
|---|---|---|
| [0001](0001-storage-architecture.md) | Storage architecture — SQLite (hot) + Parquet (cold) | Accepted |
| 0002 | Transport choice (HTTPS/WS over SSH) | Planned |
| 0003 | Language choice (Go for hub + agent) | Planned |

## When to write a new ADR

Write an ADR when you make a decision that:

- Is hard to reverse (database engine, on-wire protocol, language)
- Affects more than one module
- Future contributors will be puzzled by ("why didn't they just…?")
- Is in response to a tradeoff with no objectively correct answer

Don't write an ADR for routine choices (which library to use for X) — those belong in PR descriptions and commit messages.

## Template

When starting a new ADR, copy this skeleton:

```markdown
# ADR-NNNN: <Short title>

- **Status**: Proposed | Accepted | Superseded by ADR-XXXX
- **Date**: YYYY-MM-DD
- **Deciders**: <names>

## Context
## Decision
## Consequences
### Positive
### Negative
### Neutral
## Alternatives rejected
## References
## Related ADRs
```

Numbering: take the next free integer, four digits, zero-padded.
