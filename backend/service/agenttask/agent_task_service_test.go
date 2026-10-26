package service

import (
	"strings"
	"testing"
	"time"

	"backend/model"
)

func TestAgentTaskDefName(t *testing.T) {
	// 截断 + 换行折叠
	name := agentTaskDefName("每天早上总结新闻\n并推送给我,这是一个很长很长的输入内容超出了限制")
	if !strings.HasPrefix(name, "Agent任务-每天早上总结新闻 并推送给我,这是一个很长很长的-") {
		t.Fatalf("名称格式不符: %q", name)
	}
	if got := agentTaskDefName("   \n  "); !strings.HasPrefix(got, "Agent任务-任务-") {
		t.Fatalf("空输入应回退为占位: %q", got)
	}
}

func TestValidateAgentTaskSchedule(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	// once
	if err := validateAgentTaskSchedule(model.ScheduleTypeOnce, nil, ""); err == nil {
		t.Fatal("once 缺执行时间应报错")
	}
	if err := validateAgentTaskSchedule(model.ScheduleTypeOnce, &past, ""); err == nil {
		t.Fatal("once 过去时间应报错")
	}
	if err := validateAgentTaskSchedule(model.ScheduleTypeOnce, &future, ""); err != nil {
		t.Fatalf("合法 once 不应报错: %v", err)
	}
	// cron
	if err := validateAgentTaskSchedule(model.ScheduleTypeCron, nil, "not a cron"); err == nil {
		t.Fatal("非法 cron 应报错")
	}
	if err := validateAgentTaskSchedule(model.ScheduleTypeCron, nil, "0 9 * * *"); err != nil {
		t.Fatalf("合法 cron 不应报错: %v", err)
	}
	// 未知类型
	if err := validateAgentTaskSchedule("repeat", &future, ""); err == nil {
		t.Fatal("未知调度类型应报错")
	}
}
