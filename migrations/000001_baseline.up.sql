CREATE TABLE IF NOT EXISTS `users` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `email` varchar(191) NOT NULL,
    `password` longtext NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_users_email` (`email`),
    INDEX `idx_users_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `tournaments` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `name` longtext,
    `slug` longtext,
    `region` longtext,
    PRIMARY KEY (`id`),
    INDEX `idx_tournaments_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `teams` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `team_id` bigint,
    `name` longtext,
    `logo_url` longtext,
    `primary_color` longtext,
    `secondary_color` longtext,
    `text_color` longtext,
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_teams_team_id` (`team_id`),
    INDEX `idx_teams_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `events` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `sofa_score_event_id` bigint,
    `sport` longtext,
    `home_score` bigint,
    `home_team_id` bigint,
    `away_score` bigint,
    `away_team_id` bigint,
    `scraped_at` bigint,
    `start_timestamp` bigint,
    `current_period_start_timestamp` bigint,
    `slug` longtext,
    `league_id` bigint unsigned,
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_events_sofa_score_event_id` (`sofa_score_event_id`),
    INDEX `idx_events_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `domains` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `domain` varchar(191) NOT NULL,
    `user_id` bigint unsigned NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_domains_domain` (`domain`),
    INDEX `idx_domains_user_id` (`user_id`),
    INDEX `idx_domains_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `refresh_tokens` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `user_id` bigint unsigned NOT NULL,
    `token_id` varchar(64) NOT NULL,
    `expires_at` datetime(3) NOT NULL,
    `revoked_at` datetime(3) NULL,
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_refresh_tokens_token_id` (`token_id`),
    INDEX `idx_refresh_tokens_user_id` (`user_id`),
    INDEX `idx_refresh_tokens_expires_at` (`expires_at`),
    INDEX `idx_refresh_tokens_revoked_at` (`revoked_at`),
    INDEX `idx_refresh_tokens_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `devices` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `user_id` bigint unsigned,
    `token` varchar(191) NOT NULL,
    `platform` longtext,
    `name` longtext,
    `last_seen` bigint,
    `version` longtext,
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_devices_token` (`token`),
    INDEX `idx_devices_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `playback_logs` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `device_id` bigint unsigned NOT NULL,
    `content` longtext NOT NULL,
    `started_at` bigint,
    `ended_at` bigint,
    PRIMARY KEY (`id`),
    INDEX `idx_playback_logs_device_id` (`device_id`),
    INDEX `idx_playback_logs_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `apk_versions` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `version` varchar(191) NOT NULL,
    `file_name` longtext NOT NULL,
    `file_path` longtext NOT NULL,
    `file_size` bigint,
    `description` longtext,
    `is_active` tinyint(1) DEFAULT 1,
    `package_name` varchar(191) NOT NULL,
    `version_code` int,
    `min_sdk_version` int,
    `target_sdk_version` int,
    `download_token` varchar(191) NOT NULL,
    `total_downloads` bigint DEFAULT 0,
    `iptv_url` longtext DEFAULT 'http://5.mdtv.me',
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_apk_version_package` (`version`, `package_name`),
    UNIQUE INDEX `idx_apk_versions_download_token` (`download_token`),
    INDEX `idx_apk_versions_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `device_tournaments` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `device_id` bigint unsigned NOT NULL,
    `tournament_id` bigint unsigned NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_device_tournament` (`device_id`, `tournament_id`),
    INDEX `idx_device_tournaments_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `global_tournament_configs` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `tournament_id` bigint unsigned NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_global_tournament_configs_tournament_id` (`tournament_id`),
    INDEX `idx_global_tournament_configs_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `content_stats` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `content_hash` varchar(191) NOT NULL,
    `period_type` varchar(191) NOT NULL,
    `period_start` datetime(3) NOT NULL,
    `seconds` bigint NOT NULL,
    `views` bigint NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_content_period` (`content_hash`, `period_type`, `period_start`),
    INDEX `idx_content_stats_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `crash_reports` (
    `id` bigint unsigned AUTO_INCREMENT,
    `created_at` datetime(3) NULL,
    `updated_at` datetime(3) NULL,
    `deleted_at` datetime(3) NULL,
    `fatal` tinyint(1),
    `error` longtext,
    `stack_trace` longtext,
    `context` longtext,
    `app_name` longtext,
    `app_version` longtext,
    `app_build` longtext,
    `app_environment` longtext,
    `app_platform` longtext,
    `device_os_version` longtext,
    `device_locale` longtext,
    PRIMARY KEY (`id`),
    INDEX `idx_crash_reports_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
