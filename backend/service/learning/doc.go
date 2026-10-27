// Package learning 英语学习域: 分级阅读 / RSS 订阅 / 生词本 / 造句测验 / 课程 / 错题本 / FSRS 复习。
// 集群内互相调用密集 (reading↔rss↔vocabulary↔quiz↔course), 故合成一个包而非拆成多个子包。
// 对集群外只依赖窄接口 (interfaces.TextGenerator/UserDocumentStore/PreferenceGetter/ConfigValueGetter)
// 与共享层 (kernel/llmjson/prompt), 不 import backend/service 父包, 从根上避免循环依赖。
package learning

import "gorm.io/gorm"

// DB 本包专用数据库句柄, 由 main.go 接线赋值 (learning.DB = db)。
// 子包内部所有 DB. 调用点无需改动。
var DB *gorm.DB
