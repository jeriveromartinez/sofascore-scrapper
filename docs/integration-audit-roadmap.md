# Integration audit roadmap

This document tracks compatibility shims the mobile and web clients
need to be aware of when the wire contract changes. It is referenced
from `proto/api.proto` comments next to the deprecated fields.

## ExternalEvent backwards-compat (added 2026-09-11, PR #122 fix A2)

### Problem

The pre-FotMob wire schema exposed tag 4 as
`int64 sofa_score_event_id`. PR #118 (the "model reset" that replaced
SofaScore-derived fixtures with a generic `ExternalMatchId` string)
silently changed that tag to a string field. Already-deployed clients
built against the old schema broke because their generated code still
expected an `int64` at tag 4 and discarded the new position.

The deprecated and replacement fields are visible side by side in
`proto/api.proto`:

- Line ~272 — `int64 sofa_score_event_id_deprecated = 4;`
- Line ~291 — `string external_match_id = 20;`

### Rollout plan

1. The Go service keeps the legacy `int64` field at tag 4 — renamed to
   `sofa_score_event_id_deprecated` so the deprecation is obvious in
   both Go and TS bindings. The new `string external_match_id` lives
   at a fresh tag (20) so the deprecated `int64` keeps its wire
   position.
2. Old clients continue to read `int64` at tag 4; they keep working as
   long as the stored `ExternalMatchId` is numeric (i.e. pre-FotMob
   data).
3. New clients (Flutter `flutter-apptv`, web dashboard) read the string
   field at tag 20.
4. Every write must populate both fields so old clients stay in sync
   until the int64 at tag 4 is removed.

Removal of the `int64` at tag 4 is blocked on **every shipped client
reading tag 20**. The deprecation lives in the proto indefinitely
until that audit completes.

### Source of truth

- `proto/api.proto` is the source of truth. The Go bindings regenerate
  into `internal/gen/api/`.
- The Flutter mirror (`flutter-apptv/lib/api/`, in a separate repo)
  must mirror `proto/api.proto` verbatim. Verify with
  `make proto-verify-flutter`. If the Flutter proto drifts, copy the
  Go file into the Flutter repo and regenerate the Dart bindings there.

### Followups

- Audit release-channel statistics to confirm every active client reads
  tag 20 before scheduling the removal of tag 4.
- Once tag 20 is universal, bump the proto package and remove
  `sofa_score_event_id_deprecated` in a major-version wire change.
