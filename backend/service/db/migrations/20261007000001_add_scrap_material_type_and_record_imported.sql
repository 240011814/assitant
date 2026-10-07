-- +goose Up
-- 余料库存与切割记录两处增强:
--   cut_scrap_inventory.material_type  材料类型(来源材料规格), 与"名称"(label)分开维护, 不再混用
--   cut_record.scrap_imported          记录余料已从历史记录入库, 入库后前端不再展示入库入口

-- +goose StatementBegin
ALTER TABLE `cut_scrap_inventory`
  ADD COLUMN `material_type` VARCHAR(100) NOT NULL DEFAULT ''
    COMMENT '材料类型/来源材料规格 (如: 45#方管), 与名称分开维护' AFTER `scrap_type`;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `cut_record`
  ADD COLUMN `scrap_imported` TINYINT(1) NOT NULL DEFAULT 0
    COMMENT '该记录的余料是否已从历史记录入库';
-- +goose StatementEnd

-- 存量数据: material_type 留空由用户在余料管理页自行补填;
-- scrap_imported 保持 0, 已手工入库过的旧记录会仍显示入库入口 (再入一次也只是多登记, 无副作用)。

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `cut_record` DROP COLUMN `scrap_imported`;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `cut_scrap_inventory` DROP COLUMN `material_type`;
-- +goose StatementEnd
