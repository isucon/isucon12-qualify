DROP TABLE IF EXISTS `account`;
DROP TABLE IF EXISTS `tenant`;
DROP TABLE IF EXISTS `id_generator`;

CREATE TABLE `account` (
  `id` BIGINT UNSIGNED NOT NULL,
  `identifier` VARCHAR(191) NOT NULL UNIQUE,
  `name` VARCHAR(191) NOT NULL,
  `tenant_id` BIGINT NULL,
  `role` ENUM('admin', 'organizer', 'competitor', 'disqualified_competitor'),
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;

CREATE TABLE `account_access_log` (
  `id` BIGINT UNSIGNED NOT NULL,
  `account_id` BIGINT UNSIGNED NOT NULL,
  `competition_id` BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE (`account_id`, `competition_id`),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;

CREATE TABLE `tenant` (
  `id` BIGINT UNSIGNED NOT NULL,
  `identifier` VARCHAR(191) NOT NULL,
  `name` VARCHAR(191) NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;

CREATE TABLE `id_generator` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `stub` CHAR(1) NOT NULL DEFAULT '',
  PRIMARY KEY  (`id`),
  UNIQUE KEY `stub` (`stub`)
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;
