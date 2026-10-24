-- +goose Up

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS orch_runs (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    orch_id    BIGINT UNSIGNED NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    source     VARCHAR(16) NOT NULL,
    status     VARCHAR(16) NOT NULL,
    mode       VARCHAR(16) NOT NULL DEFAULT '',
    total_ms   BIGINT NOT NULL DEFAULT 0,
    error      TEXT NULL,
    summary    MEDIUMTEXT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_orch_runs_orch (orch_id, created_at),
    KEY idx_orch_runs_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS orch_templates (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT UNSIGNED NOT NULL,
    name        VARCHAR(128) NOT NULL,
    description VARCHAR(512) NOT NULL DEFAULT '',
    definition  MEDIUMTEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_orch_templates_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE orch_chat_states
    ADD COLUMN artifacts TEXT NULL AFTER routes;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE orch_chat_states DROP COLUMN artifacts;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS orch_templates;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS orch_runs;
-- +goose StatementEnd
