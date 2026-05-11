// Package parser 解析 todo_write 入参的 items 字符串：
// 自动识别 markdown checklist / 纯文本，输出归一化的 []Item。
// 设计依据：DESIGN.md §5 / §6.2。
package parser

import "strings"

// Normalize 统一换行符并去除尾部空白；不动行内字符。
// CRLF → LF；CR → LF；trim 尾部空白防 diff 噪声。
func Normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimRight(s, " \t\n")
}
