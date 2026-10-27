-- +goose Up

-- +goose StatementBegin
-- rss_sources 加 (user_id, url) 唯一索引, 兜底"重复订阅"并发竞态。
-- 先删同用户同 URL 的存量重复行 (保留最早一条), 否则唯一索引创建失败。
DELETE s1 FROM rss_sources s1
    INNER JOIN rss_sources s2
    ON s1.user_id = s2.user_id
       AND s1.url = s2.url
       AND s1.id > s2.id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE rss_sources
    ADD UNIQUE INDEX `uk_rss_user_url` (`user_id`, `url`);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE rss_sources DROP INDEX `uk_rss_user_url`;
-- +goose StatementEnd
