# Documentation

| Where | What it answers |
|---|---|
| [architecture.md](architecture.md) | **Where** — package map, dependency graph, invariants |
| [dogfooding.md](dogfooding.md) | **Usability** — friction and usage notes from actually using Sento |
| [exploratory-testing.md](exploratory-testing.md) | **Agent harness** — how testing sessions explore Sento and file issues |
| [flows/](flows/) | **How** — guided walkthroughs of each data flow through the code |
| [spec/](spec/) | **What & why** — product behavior and the decision log |
| [archive/](archive/) | Superseded documents, kept for the "why was this abandoned" trail |

The spec:

- [spec/spec.md](spec/spec.md) — goals, domain model, and the **decision log** (§1)
- [spec/parser.md](spec/parser.md) — release-name parser & matcher (regression corpus in `internal/parse`)
- [spec/decision-engine.md](spec/decision-engine.md) — grab decisions & release profiles

## Rules

- **Filenames, § numbers and D numbers are permanent identifiers.** Code
  comments cite them (`spec.md §11.4`, `D13`). Never rename or renumber;
  only append.
- **New decisions get logged**: add D16, D17, … to spec.md §1. Supersede
  by striking through and pointing forward (see D7 → D15), never by
  rewriting history.
- **When code and spec disagree, code wins** — fix the spec with a
  correction note.
- `spec/` documents intent, `flows/` documents the implementation; they
  cite each other, they don't duplicate each other.
