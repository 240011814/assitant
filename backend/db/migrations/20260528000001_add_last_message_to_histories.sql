-- +goose Up
ALTER TABLE training_histories ADD COLUMN last_message VARCHAR(500) DEFAULT '' AFTER messages;

-- Backfill: extract last non-system message preview from existing data
UPDATE training_histories
SET last_message = LEFT(
    TRIM(
        REPLACE(
            REPLACE(
                REPLACE(
                    JSON_UNQUOTE(
                        JSON_EXTRACT(
                            messages,
                            CONCAT('$[', JSON_LENGTH(messages) - 1, '].content')
                        )
                    ),
                    '\\n', ' '
                ),
                '\n', ' '
            ),
            '\r', ''
        )
    ),
    200
)
WHERE messages IS NOT NULL AND messages != '' AND messages != '[]';

-- +goose Down
ALTER TABLE training_histories DROP COLUMN last_message;
