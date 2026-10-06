package service

// NormalizePage 规范化页码: <1 回 1
func NormalizePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}
