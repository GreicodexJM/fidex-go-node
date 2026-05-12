# ADR-NNNN: <short title of the decision>

- **Status:** Proposed | Accepted | Rejected | Superseded by ADR-XXXX
- **Date:** YYYY-MM-DD
- **Deciders:** <names / roles>
- **Related ticket:** <FID-NN | other>

## Context

What is the forcing function? Describe the situation, the constraints, and why a decision is needed now. Be specific about the system state at the time of writing — future readers may not remember what was on fire.

Avoid solutioning here. The Context section answers "what problem are we solving and why now?", not "what are we going to do about it?".

## Decision

State the decision in the active voice, as a single declarative paragraph. Be precise enough that a developer can pattern-match the decision against the code six months from now.

If the decision has multiple parts, use a short bulleted list — but keep the prose summary above it.

## Consequences

What changes because of this decision? Cover both:

- **Positive** — the wins this unlocks (capabilities, simplifications, performance, safety).
- **Negative / costs** — the bills that come due (migration work, complexity moved elsewhere, new failure modes, things now harder to do).

Be honest about trade-offs. An ADR with only upsides is propaganda, not architecture.

## Alternatives considered

List the other options you weighed, with a one-paragraph rationale per option for why it was rejected. Even a "do nothing" alternative is worth naming explicitly.

- **Alternative A** — <rejected because…>
- **Alternative B** — <rejected because…>

## References

- Link to the driving ticket(s)
- Link to relevant code paths (`internal/foo/bar.go`)
- Link to spec sections, RFCs, vendor docs, prior ADRs

## Notes for next visitor

Optional. Anything you want the next person reading this ADR to know that wouldn't fit elsewhere — e.g., "if the Symfony 6 upgrade lands first, revisit the section on payload portability".
