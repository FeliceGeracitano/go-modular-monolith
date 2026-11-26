# 0001. Record architecture decisions

- Status: Accepted
- Date: 2026-11-26

## Context

Architecture erodes when the reasons behind it are forgotten: a rule nobody
can justify is the first to be broken. Reasons live in people's heads and in
chat history, both of which are lost.

## Decision

We will record significant architecture decisions as short Markdown files in
`docs/adr/`, numbered in order, following the format in `template.md`.

## Consequences

- A newcomer can read why the system looks the way it does.
- Changing a decision means writing a new record, which makes the change
  deliberate and reviewable.
- Records take a few minutes to write. That cost is the point: a decision not
  worth ten minutes of writing is not an architecture decision.
