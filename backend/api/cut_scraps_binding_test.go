package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"backend/model"
)

// 验证分页参数可省略 (省略时返回全部), 提供时仍受 min/max 约束
func TestCutScrapSearchParamsPaginationOptional(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 省略分页: ?scrapType=1 (此前因 min=1 对零值生效而 400)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/cut/scraps?scrapType=1", nil)
	var params model.CutScrapSearchParams
	if err := c.ShouldBindQuery(&params); err != nil {
		t.Fatalf("省略分页参数时绑定失败: %v", err)
	}
	if params.ScrapType != 1 || params.Page != 0 || params.PageSize != 0 {
		t.Fatalf("参数解析不符合预期: %+v", params)
	}

	// 提供非法分页: current=-1 (非零值) 仍应被 min=1 拒绝; size=300 仍应被 max=200 拒绝
	for _, qs := range []string{"current=-1&size=5", "current=1&size=300"} {
		c2, _ := gin.CreateTestContext(httptest.NewRecorder())
		c2.Request = httptest.NewRequest("GET", "/api/cut/scraps?"+qs, nil)
		var p2 model.CutScrapSearchParams
		if err := c2.ShouldBindQuery(&p2); err == nil {
			t.Fatalf("%s 应校验失败, 实际通过", qs)
		}
	}

	// 提供合法分页: 正常绑定
	c3, _ := gin.CreateTestContext(httptest.NewRecorder())
	c3.Request = httptest.NewRequest("GET", "/api/cut/scraps?current=2&size=50", nil)
	var p3 model.CutScrapSearchParams
	if err := c3.ShouldBindQuery(&p3); err != nil {
		t.Fatalf("合法分页参数绑定失败: %v", err)
	}
	if p3.Page != 2 || p3.PageSize != 50 {
		t.Fatalf("分页参数解析不符合预期: %+v", p3)
	}
}
