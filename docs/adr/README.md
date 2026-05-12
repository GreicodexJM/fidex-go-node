# Architecture Decision Records

This directory holds the Architecture Decision Records (ADRs) for **fidex-go-node**, the Go reference implementation of the FideX AS5 protocol.

## What is an ADR?

An ADR captures an **architecturally significant** decision: the context that forced the choice, the alternatives considered, the decision itself, and the consequences we expect to live with. ADRs are the long-lived audit trail of "why is the code shaped this way?". They complement — but do not replace — code comments, design docs, or runbooks.

We adopted the lightweight Michael Nygard format. Keep them short, durable, and honest about trade-offs.

## Convention

- Files are numbered monotonically, zero-padded to four digits: `NNNN-short-kebab-title.md`.
- `0001-template.md` is the canonical template — copy it when starting a new ADR.
- One decision per file. If a later ADR overturns an earlier one, the new ADR **supersedes** the old one and the old one's status is flipped to `Superseded by ADR-XXXX`.
- ADRs are committed to `master` like any other source artefact — they are reviewed in the PR that introduces them.

## Status lifecycle

```
Proposed  ─┬─►  Accepted  ─►  Superseded
           └─►  Rejected
```

- **Proposed** — drafted, open for discussion. The code may not yet reflect the decision.
- **Accepted** — agreed by the architecture sync (CTO + leads). The code must match (or there is a tracked follow-up to make it match).
- **Rejected** — discussed and declined. Kept on file so the same idea doesn't get re-litigated cold six months later.
- **Superseded by ADR-XXXX** — replaced by a newer ADR.

## Index

| #     | Title                                                                 | Status   |
| ----- | --------------------------------------------------------------------- | -------- |
| 0001  | Template                                                              | —        |
| 0002  | Worker job_type — column vs payload-peek dispatch                     | Proposed |
| 0003  | Dual encryption algorithm support (RSA-OAEP + RSA-OAEP-256)           | Accepted |

## Authoring tips

- State the **problem** before the decision. If the next reader can't infer the forcing function, the ADR has failed.
- List **alternatives considered**, even briefly. The rejected paths are half the value.
- Be specific about **consequences** — both wins and the bills coming due.
- Link the ticket that drove the ADR and the code paths it touches.
