# Exploratory upstream scenarios

Run the supervised `fixtures` service (`mise run explore:fixtures` with a
Paseo-assigned port). The normal binary omits this code; the fixture binary
requires `-control` to expose `/_control`. Commands here target **fixtures**,
not sandboxapi. They never alter Sento's stored state.

| Name | Scenario Source page-5 behavior, independently per chapter/ref |
| --- | --- |
| `healthy` | Five valid GIF pages per chapter. No injected failure. |
| `nested-listings` | Healthy pages; exposes the fixture extension in the picker. |
| `page-503` | First two attempts fail with upstream 503; then succeed. |
| `rate-limit` | First attempt fails with 429 and Retry-After=2 seconds. |
| `long-cooldown` | First attempt fails with 429 and Retry-After=60 seconds. |
| `permanent-401` | Every attempt fails with upstream 401, retryable=false. |
| `source-outage` | Every attempt fails with upstream 503. |
| `truncated-page` | First two HTTP 200 bodies end before Content-Length. |

The fixture uses the sidecar's gateway 502 envelope with upstreamStatus,
retryable and retryAfter. Faults affect only `scenario-en-1`; the old stub
sources keep their normal behavior. Root and nested directory fixtures are
available in every scenario. Series 105/106 are searchable when controls are
enabled. Three chapter releases have distinct page refs so attempts do not leak
from one chapter into another. Existing attempts are intentionally retained
when selecting another scenario; “first attempt” means since the last reset.

- `GET /_control`: service marker, current/available scenarios, per-ref attempt
  counts, bounded timestamped event journal (latest 5,000), independent regular
  and VPN torrent maps. Credentials and arbitrary request bodies are omitted.
- `POST /_control/scenario` with `{"name":"page-503"}`: choose the fault plan;
  preserve attempt counts, torrent maps and evidence.
- `POST /_control/reset`: healthy plan, empty attempts/journal/torrents. Stop
  sandboxapi and capture evidence first; do not reset an observed retry.
- `POST /_control/torrent`: model a fake client's state using e.g.
  `{"client":"vpn","hash":"<observed hash>","name":"Test release","category":"sento-test","state":"stoppedDL","progress":0.2}`.
  It replaces that torrent's simulated state. `progress:1` marks complete but
  invents no files. Finished payload/file-import testing uses real local files.

Client endpoints `/qbit` and `/qbit-vpn` accept magnets, remember category,
return hashes/progress, and journal exact deletion hashes and `deleteFiles`.
The Torznab fixture `/torznab/api` supplies three volume releases with stable
hashes ending 1, 2, 3. Different series share these fixture hashes: finish or
cancel/clean up one routing mission before starting another. This deliberate
small universe is not a general-purpose torrent server; APK loading, actual
tracker/VPN connectivity and Madokami HTML parsing require separate contract or
live smoke checks. No fake torrent transfers or deletes actual disk data.
