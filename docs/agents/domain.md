# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- `CONTEXT.md` at the repo root for the TokenInsights domain glossary.
- `docs/design.md` for the architecture, schema contract, pipeline, viewer, and invariant contract.
- `docs/adr/` for decisions that touch the area being changed.

If any of these files don't exist, proceed silently. The producer skill (`grill-with-docs`) creates glossary and ADR files lazily when terms or decisions actually get resolved.

## File structure

This is a single-context repo:

```text
/
├── CONTEXT.md
├── docs/design.md
├── docs/adr/
└── packages/
```

## Use the glossary's vocabulary

When output names a domain concept in an issue title, refactor proposal, hypothesis, or test name, use the term as defined in `CONTEXT.md`. Do not drift to synonyms the glossary explicitly avoids.

If the concept is missing from the glossary, either reconsider whether the wording belongs to this project or note it for `grill-with-docs`.

## Flag ADR conflicts

If output contradicts an existing ADR, surface it explicitly rather than silently overriding it.
