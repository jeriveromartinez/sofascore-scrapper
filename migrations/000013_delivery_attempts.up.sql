-- delivery_attempts is one row per (push, device) — the snapshot of
-- the audience at fire time. state evolves sent -> delivered | failed
-- as the lifecycle progresses. latency_ms is set on the delivered
-- transition (sent_at -> acked_at) and feeds the p50/p95 gauges.
--
-- The unique key (push_message_id, device_id) makes a re-fire of the
-- same push idempotent at the row level.
CREATE TABLE delivery_attempts (
  id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  created_at        TIMESTAMP NOT NULL,
  updated_at        TIMESTAMP NOT NULL,
  push_message_id   BIGINT UNSIGNED NOT NULL,
  device_id         BIGINT UNSIGNED NOT NULL,
  state             VARCHAR(16) NOT NULL,
  failure_reason    VARCHAR(32) NULL,
  sent_at           TIMESTAMP NULL,
  acked_at          TIMESTAMP NULL,
  latency_ms        INT NULL,
  FOREIGN KEY (push_message_id) REFERENCES push_messages(id) ON DELETE CASCADE,
  FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
  UNIQUE KEY uq_push_device (push_message_id, device_id),
  INDEX idx_attempts_state_created (state, created_at),
  INDEX idx_attempts_device_created (device_id, created_at)
);
