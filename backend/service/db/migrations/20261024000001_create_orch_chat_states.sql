-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS orch_chat_states (
    history_id BIGINT UNSIGNED PRIMARY KEY,
    orch_id    BIGINT UNSIGNED NOT NULL,
    active     TINYINT(1) NOT NULL DEFAULT 0,
    routes     TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_orch_chat_states_orch_id (orch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS orch_chat_states;
-- +goose StatementEnd
