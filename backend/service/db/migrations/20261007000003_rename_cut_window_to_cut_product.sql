-- +goose Up

-- +goose StatementBegin
-- 窗户单更名产品单 (后续会导入窗以外的产品类型): 表名同步更名, spec JSON 结构由前端兼容解析
RENAME TABLE `cut_window` TO `cut_product`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
RENAME TABLE `cut_product` TO `cut_window`;
-- +goose StatementEnd
