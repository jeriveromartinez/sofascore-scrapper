-- push_messages is the header of a single push delivery. The payload
-- (title, body, image_url, deep_link, priority, ttl, data) is
-- denormalized so delivery time needs zero joins.
--
-- push_message_targets is the explicit join table to the audience
-- domains (multidominio). Modeled explicitly (not a GORM-only
-- auto-table) so we can add metadata columns later (e.g. "sent_at"
-- per target) without a migration.
CREATE TABLE push_messages (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  created_at      TIMESTAMP NOT NULL,
  updated_at      TIMESTAMP NOT NULL,
  user_id         BIGINT UNSIGNED NOT NULL,
  category        VARCHAR(32)  NOT NULL,
  title           VARCHAR(200) NOT NULL,
  body            VARCHAR(2000) NOT NULL,
  image_url       VARCHAR(500) NULL,
  deep_link       VARCHAR(500) NULL,
  priority        VARCHAR(16)  NOT NULL DEFAULT 'normal',
  ttl_seconds     INT          NOT NULL DEFAULT 0,
  data_json       JSON         NULL,
  source          VARCHAR(16)  NOT NULL,
  scheduled_id    BIGINT UNSIGNED NULL,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  INDEX idx_push_messages_user_created (user_id, created_at)
);

CREATE TABLE push_message_targets (
  push_message_id BIGINT UNSIGNED NOT NULL,
  domain_id       BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (push_message_id, domain_id),
  FOREIGN KEY (push_message_id) REFERENCES push_messages(id) ON DELETE CASCADE,
  FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
);
