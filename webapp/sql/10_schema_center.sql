DROP TABLE IF EXISTS `account`;
DROP TABLE IF EXISTS `tenant`;
DROP TABLE IF EXISTS `id_generator`;

CREATE TABLE `account` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `identifier` VARCHAR(191) NOT NULL UNIQUE,
  `name` VARCHAR(191) NOT NULL,
  `image` LONGBLOB NOT NULL,
  `tenant_id` BIGINT NULL,
  `role` ENUM('saas_operator', 'tenant_admin', 'competitor'),
  `last_accessed_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;

CREATE TABLE `tenant` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `identifier` VARCHAR(191) NOT NULL,
  `name` VARCHAR(191) NOT NULL,
  `image` LONGBLOB NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;

CREATE TABLE `id_generator` (
  `id` bigint UNSIGNED NOT NULL AUTO_INCREMENT,
  `stub` char(1) NOT NULL DEFAULT '',
  PRIMARY KEY  (`id`),
  UNIQUE KEY `stub` (`stub`)
) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4;
