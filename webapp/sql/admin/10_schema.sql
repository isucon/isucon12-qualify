USE `isuports`;

DROP TABLE IF EXISTS `tenant`;
DROP TABLE IF EXISTS `id_generator`;
DROP TABLE IF EXISTS `visit_history`;

CREATE TABLE `tenant` (
  `id` BIGINT UNSIGNED NOT NULL,
  `name` VARCHAR(256) NOT NULL,
  `display_name` VARCHAR(256) NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;

CREATE TABLE `id_generator` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `stub` CHAR(1) NOT NULL DEFAULT '',
  PRIMARY KEY  (`id`),
  UNIQUE KEY `stub` (`stub`)
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;

CREATE TABLE `visit_history` (
  `player_name` VARCHAR(256) NOT NULL,
  `tenant_id` BIGINT UNSIGNED NOT NULL,
  `competition_id` BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  INDEX `tenant_id_idx` (`tenant_id`, `competition_id`, `created_at`),
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;

CREATE TABLE `visit_history_s` (
  `player_name` varchar(256) NOT NULL,
  `tenant_id` bigint unsigned NOT NULL,
  `competition_id` bigint unsigned NOT NULL,
  `min_created_at` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`tenant_id`,`competition_id`,`player_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS billing_report (
  tenant_id bigint unsigned NOT NULL,
  competition_id bigint unsigned NOT NULL,
  competition_title TEXT NOT NULL,
  player_count bigint unsigned NOT NULL,
  billing_yen bigint unsigned NOT NULL,
  PRIMARY KEY (tenant_id, competition_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
