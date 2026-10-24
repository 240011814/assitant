package service

import (
	"log"
	"runtime/debug"
)

// SafeGo 后台协程统一封装: recover panic 防止拖垮整个进程 (HTTP 层有 gin Recovery 兜底,
// 手工启动的后台协程没有), 并带上调用点名与堆栈留痕定位。
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[SafeGo] %s panic: %v\n%s", name, r, debug.Stack())
			}
		}()
		fn()
	}()
}
