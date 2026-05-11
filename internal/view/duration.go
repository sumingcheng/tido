package view

import (
	"fmt"
	"time"
)

// relativeDue 把截止时间渲染为相对时间字符串（compact 模式）。
//
//	@5m  / @2h  / @3d           — 未来
//	@overdue 1d / @overdue 2h   — 已过期
//	@now                        — 极近
func relativeDue(dueMs, nowMs int64) string {
	d := time.Duration(dueMs-nowMs) * time.Millisecond

	if d.Abs() < time.Minute {
		return "@now"
	}
	if d < 0 {
		return "@overdue " + humanizeDuration(-d)
	}
	return "@" + humanizeDuration(d)
}

// humanizeDuration 把时长压成最大单位的整数+单位（向下取整）。
func humanizeDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
}
