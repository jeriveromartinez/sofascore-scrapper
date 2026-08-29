-- scheduled_pushes is the cron-like program table. The runner polls on
-- next_fire_at (indexed) and either fires (one_shot: is_active=false,
-- last_fired_at=now; recurring: next_fire_at=cron.Next(now, expr)) or
-- skips an inactive row.
--
-- The payload fields mirror push_messages (one_shot and recurring use
-- the same shape) and are denormalized for the same reason: the
-- runner constructs a push_messages row from this denormalized payload
-- at fire time.
--
-- scheduled_push_targets is the explicit join to the audience domains.
CREATE TABLE scheduled_pushes (
  id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  created_at        TIMESTAMP NOT NULL,
  updated_at        TIMESTAMP NOT NULL,
  user_id           BIGINT UNSIGNED NOT NULL,
  schedule_type     VARCHAR(16)  NOT NULL,
  run_at            TIMESTAMP NULL,
  cron_expr         VARCHAR(64)  NULL,
  next_fire_at      TIMESTAMP NOT NULL,
  last_fired_at     TIMESTAMP NULL,
  is_active         BOOLEAN     NOT NULL DEFAULT TRUE,
  category          VARCHAR(32) NOT NULL,
  title             VARCHAR(200) NOT NULL,
  body              VARCHAR(2000) NOT NULL,
  image_url         VARCHAR(500) NULL,
  deep_link         VARCHAR(500) NULL,
  priority          VARCHAR(16)  NOT NULL DEFAULT 'normal',
  ttl_seconds       INT          NOT NULL DEFAULT 0,
  data_json         JSON         NULL,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  INDEX idx_schedules_active_next (is_active, next_fire_at)
);

CREATE TABLE scheduled_push_targets (
  scheduled_push_id BIGINT UNSIGNED NOT NULL,
  domain_id         BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (scheduled_push_id, domain_id),
  FOREIGN KEY (scheduled_push_id) REFERENCES scheduled_pushes(id) ON DELETE CASCADE,
  FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
);
