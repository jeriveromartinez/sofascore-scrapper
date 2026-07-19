CREATE TABLE IF NOT EXISTS apk_upload_publications (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    upload_id CHAR(36) NOT NULL,
    temp_path VARCHAR(1024) NOT NULL,
    final_path VARCHAR(1024) NOT NULL DEFAULT '',
    status ENUM('assembling','published','persisted','failed') NOT NULL DEFAULT 'assembling',
    user_id INT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX idx_upload_id (upload_id),
    INDEX idx_status (status),
    INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
