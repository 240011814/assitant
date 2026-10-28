-- +goose Up

-- RSS 中英对照: 段落级分批 LLM 翻译, 服务端组装"原文块 + 译文块"的双语 HTML 缓存到条目。
-- translation_hash 是翻译时的 content_html 指纹: 原文重新抓取后内容变化即视为译文过期。

-- +goose StatementBegin
ALTER TABLE `rss_items`
    ADD COLUMN `bilingual_html` MEDIUMTEXT NULL COMMENT '中英对照 HTML (原文块 + rss-zh 译文块)' AFTER `content_html`,
    ADD COLUMN `translation_hash` VARCHAR(64) NULL COMMENT '翻译时的 content_html 指纹' AFTER `bilingual_html`,
    ADD COLUMN `translated_at` DATETIME NULL AFTER `translation_hash`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `rss_items`
    DROP COLUMN `translated_at`,
    DROP COLUMN `translation_hash`,
    DROP COLUMN `bilingual_html`;
-- +goose StatementEnd
