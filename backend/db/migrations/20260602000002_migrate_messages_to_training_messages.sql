-- +goose Up
-- +goose StatementBegin
INSERT INTO training_messages (history_id, role, content, sort_order, created_at)
SELECT
    th.id AS history_id,
    JSON_UNQUOTE(JSON_EXTRACT(msg.val, '$.role')) AS role,
    JSON_UNQUOTE(JSON_EXTRACT(msg.val, '$.content')) AS content,
    msg.idx AS sort_order,
    th.created_at
FROM training_histories th,
JSON_TABLE(
    th.messages,
    '$[*]' COLUMNS (
        idx FOR ORDINALITY,
        val JSON PATH '$'
    )
) AS msg
WHERE th.messages IS NOT NULL AND th.messages != '' AND th.messages != '[]';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE training_histories DROP COLUMN messages;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE training_histories ADD COLUMN messages LONGTEXT;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE training_histories th
SET messages = (
    SELECT CONCAT('[', GROUP_CONCAT(
        JSON_OBJECT('role', tm.role, 'content', tm.content)
        ORDER BY tm.sort_order
    ), ']')
    FROM training_messages tm
    WHERE tm.history_id = th.id
)
WHERE EXISTS (
    SELECT 1 FROM training_messages tm WHERE tm.history_id = th.id
);
-- +goose StatementEnd
