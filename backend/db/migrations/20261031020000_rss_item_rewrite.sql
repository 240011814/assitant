-- +goose Up

-- RSS 分级改写: 把原文按用户当前 CEFR 档位改写成可读版本, 缓存到条目。
-- rewrite_hash 是改写时的 content_html 指纹: 原文重新抓取后内容变化即视为改写过期。
-- rewrite_level 记录改写目标档位 (来自词汇量校准), 供前端展示与后续阶梯升级判断。

-- +goose StatementBegin
ALTER TABLE `rss_items`
    ADD COLUMN `rewrite_html` MEDIUMTEXT NULL COMMENT '按用户档位改写后的 HTML (p/pre 段落序列)' AFTER `translation_hash`,
    ADD COLUMN `rewrite_level` VARCHAR(4) NULL COMMENT '改写目标 CEFR 档位' AFTER `rewrite_html`,
    ADD COLUMN `rewrite_hash` VARCHAR(64) NULL COMMENT '改写时的 content_html 指纹' AFTER `rewrite_level`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `rss_items`
    DROP COLUMN `rewrite_hash`,
    DROP COLUMN `rewrite_level`,
    DROP COLUMN `rewrite_html`;
-- +goose StatementEnd
