# Frontend UX review

Reviewed October 5, 2026. Scope: the library, adding and importing series, series details, release search, activity, calendar, settings, profiles, indexers, and system tasks. Mantine remains the UI library.

## Direction

Sento should feel like a practical library manager. The user’s main questions are: what is missing, what is downloading, what failed, and what needs configuring? Covers provide the visual interest; neutral surfaces, restrained accents, readable spacing, and explicit actions provide the structure. Avoid promotional empty states, invented terminology, decorative status charts, and claims of success before a request completes.

Mantine already provides the needed building blocks: responsive app shell, tabs, forms, accessible dialogs, scroll containers, loading states, notifications, and theme variables. Shared component defaults and CSS modules make the visual treatment consistent without adding a replacement library.

## Findings and implemented changes

| Area | Finding | Change and purpose |
| --- | --- | --- |
| Activity | Long errors and unbroken URLs widened the entire page. The important download state became difficult to scan. | A fixed-layout desktop table and mobile record cards keep each item contained. Errors have a wrapping two-line preview; View error opens a scrollable, wrapping dialog with Copy error. Long titles are limited in the summary. |
| Activity organization | Active downloads, completed imports, and blocked releases competed for attention. A summary chart added little useful information. | Queue, Completed, and Blocked releases are separate tabs. Failed downloads remain in the queue with a clear attention message. Progress and status stay beside the release. |
| Activity actions | “Process” did not explain the user-visible effect. | Check downloads explains that finished downloads are checked and available files imported. Blocking a failed release requires confirmation and explains how to undo it. |
| Library | Selection could outlive the visible filtered set. Bulk deletion could affect titles the user could no longer see. | Changing filters or layout clears selection. Bulk actions use the visible selection. Removal confirms the count and explains that files stay on disk. |
| Library presentation | Heavy cards and decorative treatment distracted from covers. | A simple poster grid, compact filters, explicit table/grid controls, and a functional empty state keep the collection central. Filters work together and can be cleared. |
| Adding series | Search, pending results, existing-library state, and add progress could be ambiguous. | Submit with Enter, loading and retry feedback, clear existing entries, and per-item add progress. Add is unavailable while required library/profile data is unavailable. |
| Importing folders | The suggested match effectively decided what could be imported. A failed import was difficult to retry. | Users can choose another candidate or clear the match. Suggestions are labelled separately. Failed imports offer Retry import; successful rows offer Open. Import from disk can be linked directly with `/add?tab=import`. |
| Series details | Dense controls, long descriptions, and internal terminology obscured the main tasks. | A responsive cover/title header, expandable description, compact ownership statistics, and explicit Search & download / Refresh metadata actions. Monitoring and destructive actions have clear controls. |
| Series navigation | Some actionable rows and headings depended on mouse clicks. Several data sections could look empty while still loading. | Slot buttons and expandable collection headers support the keyboard. Tables scroll within their own containers. Query failures have retry feedback. Inactive tabs do not mount their data views. |
| Release search | Download state could remain unclear after a click; forced downloads needed an understandable consequence. | Successful candidates show Queued and disable repeat requests. Download anyway explains the rejected rules. Search can be stopped, and closing a search aborts its request. Failed searches do not masquerade as empty results. |
| Settings | Navigating away silently discarded connection/media edits. Editing while saving could race the submitted values. | Navigation and browser unload protect dirty connection/media forms. Keep editing and Discard changes are explicit choices. Forms disable their fields during saving; dirty state stays visible. |
| Media settings | Fixed-width columns overflowed small screens. Template syntax dominated the page. | Responsive columns collapse on narrow screens; naming tokens sit in an accordion beside the preview. Errors and long previews wrap. |
| Profiles | Internal score sentinels and “ledger” wording leaked into configuration. Deletion effects were unclear. | A minimum-score switch expresses the setting directly. Download rules uses task-oriented wording. Profile/format deletion has confirmation and accurate consequences: profiles in use cannot be deleted; deleting a format removes its scores. |
| Calendar | An empty calendar and a failed request looked similar. UTC date selection could mark the wrong local day. | Separate loading, error/retry, and empty states. Today follows the local date; entries sort by date. |
| Shell | A healthy indicator could appear before health was known. Navigation used vague configuration wording. | Server status comes from loaded health data. Settings replaces Configure. Mobile search has a compact accessible control; the library stays selected on series detail routes. |
| Cross-page copy | Promotional phrases and implementation terms made routine tasks feel generated. | Functional empty states and action verbs replace phrases such as “A home for your manga”, “grabbable”, and “ledger rules”. Indexer remains because it is a meaningful domain concept in this type of application. |

## Organization decisions

Library is the home and the route into a series. Calendar answers when releases are expected. Activity answers what is happening and what needs attention. Adding a new title and importing files are distinct tabs of Add series. Settings retain separate pages for connections, media, profiles, and indexers because these configure different parts of the pipeline; merging them into one large form would make recovery and saving less clear. System remains for diagnostics and background tasks.

The series page separates overview, files/slots, history, and source matches. Labels explain the task where automatic search can initiate downloads. Destructive actions sit away from primary actions and describe whether they remove only a library record or actual files.

## Remaining opportunities

These are follow-up findings, not promises of implemented behavior:

1. **Queue API and recovery:** the current history endpoint returns the latest 200 queue items. Pagination and a backend-supported retry/cancel workflow would make Activity more complete. The frontend currently offers the recovery operations the API actually supports; it does not invent a retry button for downloads.
2. **First-run setup:** a short setup flow could explain which connection is required for torrents versus extension sources, then offer a first series or disk import. This needs agreed setup-completion rules rather than inferred connection health.
3. **Modal draft protection:** the navigation guard covers connection/media settings. Profile, indexer, and collection editors would benefit from the same deliberate dirty-draft handling when closing dialogs.
4. **Collections:** merge/delete flows and collection configuration remain more technical than the main series workflow. They deserve a focused pass, especially with several editions and long collection names.
5. **Advanced profile conditions:** custom-format condition editors remain specialist controls. A small set of examples and progressive disclosure would reduce the initial learning cost without hiding their capabilities.
6. **System hierarchy:** routine task controls could be more prominent than performance charts. Diagnose this with a real instance and operational failures before removing useful metrics.
7. **Large datasets:** test pagination, filtering speed, and selection with a much larger real library. Small mocked fixtures establish interaction correctness but do not establish performance at scale.

## Validation

The production build, ESLint, and all 35 Chromium browser tests pass. Regression coverage includes long Activity errors at 390px and 1280px, full error wrapping, completed-item separation, failed-request recovery, settings navigation protection, manual import matching and retry, filtered bulk selection, keyboard slot access, mobile series layout, and light/dark library layouts. Screenshots were reviewed for Activity on mobile.

Tests run against an isolated Playwright database and stub services. They do not establish real provider reliability or test every page at every viewport. The production build still reports its existing large-bundle warning; further chart/vendor splitting is a separate performance improvement.
