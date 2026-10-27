-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `lottery_activity` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `name` VARCHAR(100) NOT NULL COMMENT '活动名称',
    `description` TEXT COMMENT '活动描述',
    `start_time` DATETIME NOT NULL COMMENT '开始时间',
    `end_time` DATETIME NOT NULL COMMENT '结束时间',
    `status` TINYINT DEFAULT 0 COMMENT '状态: 0-未开始, 1-进行中, 2-已结束',
    `max_participants` INT DEFAULT 0 COMMENT '最大参与人数, 0-不限制',
    `current_participants` INT DEFAULT 0 COMMENT '当前参与人数',
    `points_cost` INT DEFAULT 0 COMMENT '每次抽奖消耗积分',
    `daily_limit` INT DEFAULT 0 COMMENT '每日抽奖次数限制, 0-不限制',
    `total_limit` INT DEFAULT 0 COMMENT '总抽奖次数限制, 0-不限制',
    `created_by` BIGINT UNSIGNED NOT NULL COMMENT '创建者ID',
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_status` (`status`),
    INDEX `idx_created_by` (`created_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `lottery_prize` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `activity_id` BIGINT UNSIGNED NOT NULL COMMENT '活动ID',
    `name` VARCHAR(100) NOT NULL COMMENT '奖品名称',
    `description` TEXT COMMENT '奖品描述',
    `image_url` VARCHAR(500) COMMENT '奖品图片',
    `prize_type` TINYINT DEFAULT 0 COMMENT '奖品类型: 0-实物, 1-虚拟, 2-积分',
    `prize_value` DECIMAL(10,2) DEFAULT 0 COMMENT '奖品价值',
    `total_count` INT DEFAULT 0 COMMENT '奖品总数',
    `remaining_count` INT DEFAULT 0 COMMENT '剩余数量',
    `probability` DECIMAL(5,4) DEFAULT 0 COMMENT '中奖概率 (0-1)',
    `sort_order` INT DEFAULT 0 COMMENT '排序',
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_activity_id` (`activity_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `lottery_record` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `activity_id` BIGINT UNSIGNED NOT NULL COMMENT '活动ID',
    `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
    `prize_id` BIGINT UNSIGNED COMMENT '奖品ID, NULL-未中奖',
    `prize_name` VARCHAR(100) COMMENT '奖品名称',
    `is_winner` TINYINT DEFAULT 0 COMMENT '是否中奖: 0-未中奖, 1-中奖',
    `points_cost` INT DEFAULT 0 COMMENT '消耗积分',
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_activity_id` (`activity_id`),
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_is_winner` (`is_winner`),
    INDEX `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `lottery_record`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `lottery_prize`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `lottery_activity`;
-- +goose StatementEnd
