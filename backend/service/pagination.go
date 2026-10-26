package service

import "backend/service/kernel"

// NormalizePage 委托给 kernel.NormalizePage (原 pagination.go, 抽到共享层)。
var NormalizePage = kernel.NormalizePage
