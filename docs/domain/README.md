# Domain guides

These guides describe the implemented model. [How Sento works](../README.md#how-sento-works)
summarizes the end-to-end path first. Start here with editions: the meaning
of a book, version and owned file determines how search and import work.

- [Editions, books, versions and files](editions.md): identity, collection and ownership.
- [Acquisition](acquisition.md): search, decisions, downloads, import and recovery.
- [Profiles and acquisition state](profiles-ledger.md): format matching, scoring and wanted books.
- [Parsing and title matching](parsing.md): turning release names into structured facts.
- [Naming, folders and names in readers](naming.md): where a file lands, templates and tokens, reader names and organizing existing files.
- [File metadata](metadata.md): ComicInfo fields, Sento notes and rescanning files.
- [Import, review and settlement](import-review.md): what a finished download becomes, proposals and the choices protocol.
- [Library files](library-files.md): fingerprints, scanning, presence, bulk import, cleanup, trash and removal.
- [Data model and persistence](data-model.md): the database schema, store conventions, JSON columns and state values.
- [Connections, indexers, extensions and health](connections.md): service settings, indexer entries, the extension reconcile and the health checks.

The [specifications](../spec/) record contracts and design decisions. Current
bugs and proposals belong in issues.
