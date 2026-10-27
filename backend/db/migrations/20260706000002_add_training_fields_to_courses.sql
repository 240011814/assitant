-- +goose Up
-- +goose StatementBegin
CREATE TABLE `user_course_trainings` (
  `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `course_id` BIGINT UNSIGNED NOT NULL,
  `training_status` VARCHAR(20) NOT NULL DEFAULT 'not_started',
  `training_count` INT NOT NULL DEFAULT 0,
  `last_trained_at` DATETIME DEFAULT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_user_course` (`user_id`, `course_id`),
  INDEX `idx_user_course_training_user_id` (`user_id`),
  INDEX `idx_user_course_training_course_id` (`course_id`),
  INDEX `idx_user_course_training_status` (`training_status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `user_course_trainings`;
-- +goose StatementEnd