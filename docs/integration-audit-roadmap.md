# Integration audit roadmap

This document tracks compatibility shims the mobile and web clients need to
be aware of when the wire contract changes. It is referenced from
`proto/api.proto` comments next to the deprecated fields.

## ExternalEvent backwards-compat (added 2026-09-11, PR #122 fix A2)

### Problem

The pre-FotMob wire schema exposed tag 4 as `int64 sofa_score_event_id`.
PR #118 (the "model reset" that replaced SofaScore-derived fixtures with a
generic `ExternalMatchId` string) silently changed that tag to a string
field. Already-deployed clients built against the old schema broke: their
generated code still expected an int64 at tag 4 and discarded the new
position.

### Rollout plan

1. The Go service keeps the legacy `int64 sofa_score_event_id_deprecated`
   field at tag 4 (renamed to make the deprecation obvious in the Go and
   TS bindings). The new `string external_match_id` lives at a fresh tag
   (20) so the deprecated int64 keeps its wire position.
2. Old clients continue to read int64 at tag 4; they keep working as long
   as the stored `ExternalMatchId` is numeric (i.e. pre-FotMob data).
3. New clients (Flutter apptv, web dashboard) read the string field at
   tag 20.
4. Once every shipped client reads from tag 20, the deprecated int64 at
   tag 4 can be removed in a future major-version bump. Until then,
   every write must populate both fields so old clients stay in sync.

### Followups

- Audit release channel statistics to confirm every active client reads
  tag 20 before scheduling the removal of tag 4.
- Document the deprecation in the Flutter client's `proto/api.proto`
  mirror once the audit lands.