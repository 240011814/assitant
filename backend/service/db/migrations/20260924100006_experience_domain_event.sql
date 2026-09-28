-- +goose Up

-- +goose StatementBegin
-- 经历改用两级维度: domain(领域) + event_type(事件类型), 与画像维度解耦; 彻底废弃 category
ALTER TABLE user_experiences
    ADD COLUMN domain VARCHAR(32) NOT NULL DEFAULT '' COMMENT '事件领域: career/education/project/skill/technology/finance/health/lifestyle/relationship/community/legal/travel/hobby/habit/personality/preference/achievement/challenge/other';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE user_experiences
    ADD COLUMN event_type VARCHAR(32) NOT NULL DEFAULT '' COMMENT '事件类型: start/ongoing/complete/achieve/fail/abandon/decide/change/participate/publish/compete/volunteer/relocate/recover/experiment/maintain';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE user_experiences
    ADD COLUMN importance TINYINT NOT NULL DEFAULT 3 COMMENT '重要度 1~5';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE user_experiences
    DROP COLUMN category;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE user_experiences
    ADD COLUMN category VARCHAR(32) NOT NULL DEFAULT '' COMMENT 'work/project/study/achievement/challenge/other';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE user_experiences
    DROP COLUMN importance,
    DROP COLUMN event_type,
    DROP COLUMN domain;
-- +goose StatementEnd