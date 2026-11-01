# Documentation

Start with the guide for what you want to do:

| Guide | Contents |
| --- | --- |
| [Installation](installation.md) | Containers, binaries, configuration, storage and upgrades |
| [Development](development.md) | Local setup, dev services and common tasks |
| [Testing](testing/README.md) | Go, PostgreSQL, browser and sidecar checks |
| [Releases](releases.md) | Conventional Commits, version policy and publishing |
| [Architecture](architecture.md) | Package map and dependency boundaries |
| [Domain](domain) | Editions, acquisition, parsing and library behavior |
| [Specifications](spec) | Product decisions, release parser and decision engine |
| [Exploratory testing](exploratory-testing.md) | Testing missions and controllable upstreams |

`domain/` describes implemented behavior. `spec/` records intent and decisions.
If a spec disagrees with current code, verify the behavior and document the
correction. Code comments cite spec filenames, section numbers and decision
IDs; preserve those identifiers when editing. Append new decisions and mark
superseded ones with a link to their replacement.

Track bugs and proposals in GitHub issues. Keep setup instructions in the
guides above and behavior rules next to their domain documentation.
