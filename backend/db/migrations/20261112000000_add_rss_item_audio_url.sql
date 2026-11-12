-- +goose Up

-- RSS 播客条目识别: feed 里带 mp3/m4a enclosure 的条目 (播客节目) 记录音频地址,
-- 阅读器对这类条目显示收听条 + "本期为播客节目"提示, 而不是当成缺正文。

-- +goose StatementBegin
ALTER TABLE `rss_items`
    ADD COLUMN `audio_url` VARCHAR(500) NULL COMMENT '播客音频地址 (feed 的 mp3/m4a enclosure)' AFTER `link`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `rss_items`
    DROP COLUMN `audio_url`;
-- +goose StatementEnd
