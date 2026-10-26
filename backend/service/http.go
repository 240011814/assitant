package service

import (
	"time"
	"net/http"

	"backend/service/kernel"
)

// newHTTPClient 委托给 kernel.NewHTTPClient (原 http.go, 抽到共享层以免业务子包循环依赖)。
// TimeoutConfig 已是 kernel.TimeoutConfig 的真别名, 签名与调用点不变。
var newHTTPClient = func(timeout time.Duration, config ...TimeoutConfig) *http.Client {
	return kernel.NewHTTPClient(timeout, config...)
}
