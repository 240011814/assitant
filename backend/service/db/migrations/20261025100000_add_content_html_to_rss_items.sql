-- +goose Up

-- +goose StatementBegin
-- RSS 条目的净化 HTML 正文 (阅读视图渲染用; content 仍为纯文本, 供导入/标注/听写)
ALTER TABLE rss_items
    ADD COLUMN content_html MEDIUMTEXT NULL COMMENT '净化后的 HTML 正文';
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE rss_items DROP COLUMN content_html;
-- +goose StatementEnd
