-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `users` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `username` VARCHAR(50) NOT NULL UNIQUE,
    `password_hash` VARCHAR(255) NOT NULL,
    `nickname` VARCHAR(100),
    `role` VARCHAR(20) DEFAULT 'R_USER',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
-- Default admin user: admin / admin123
INSERT INTO `users` (`username`, `password_hash`, `nickname`, `role`) 
VALUES ('admin', '$2a$10$dLCff5.Ysbdj4/Yfus4frOPP9oPEky82gIHA7yK2fhSfqHHM.KIFW', '管理员', 'R_SUPER')
ON DUPLICATE KEY UPDATE `username`=`username`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `users`;
-- +goose StatementEnd
