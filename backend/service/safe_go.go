package service

import "backend/service/kernel"

// SafeGo 委托给 kernel.SafeGo (原 safe_go.go, 抽到共享层以免业务子包循环依赖)。
// 保持 service 包内的调用点不变。
var SafeGo = kernel.SafeGo
