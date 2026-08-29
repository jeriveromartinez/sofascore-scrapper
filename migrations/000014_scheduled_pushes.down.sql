-- Revert scheduled_pushes + scheduled_push_targets.
DROP TABLE IF EXISTS scheduled_push_targets;
DROP TABLE IF EXISTS scheduled_pushes;
