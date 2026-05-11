// Package validate 提供输入字符串的语义层校验：UTF-8、控制字符、长度、字符集白名单。
// schema validate 拦截类型/pattern/enum；本包做 schema 表达不了的语义检查。
// 设计依据：DESIGN.md §6.1。
package validate

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// 长度限制常量（DESIGN.md §6.1）。
const (
	MaxContentBytes = 4096
	MaxContentRunes = 2000
	MaxNoteBytes    = 8192
)

// 错误集合：调用方可用 errors.Is 判别类别。
var (
	ErrEmpty       = errors.New("content is empty")
	ErrNotUTF8     = errors.New("content is not valid UTF-8")
	ErrContainsNUL = errors.New("content contains NUL byte")
	ErrControlChar = errors.New("content contains forbidden control character")
	ErrTooLongByte = errors.New("content exceeds byte limit")
	ErrTooLongRune = errors.New("content exceeds rune limit")
)

// Content 是通用校验：UTF-8 / 无 NUL / 无禁用控制字符 / ≤ maxBytes。
// 允许 \n \t 通过；禁止其他 < 0x20、DEL (0x7F) 与 ANSI escape (\x1b)。
func Content(s string, maxBytes int) error {
	if len(s) == 0 {
		return ErrEmpty
	}
	if len(s) > maxBytes {
		return fmt.Errorf("%w: %d > %d", ErrTooLongByte, len(s), maxBytes)
	}
	if !utf8.ValidString(s) {
		return ErrNotUTF8
	}
	if strings.ContainsRune(s, 0) {
		return ErrContainsNUL
	}
	for _, r := range s {
		if isForbiddenControl(r) {
			return fmt.Errorf("%w: U+%04X", ErrControlChar, r)
		}
	}
	return nil
}

// TodoContent 在 Content 基础上加 rune 上限（防恶意构造 4 字节 rune 撑爆 2000 rune 软墙）。
func TodoContent(s string) error {
	if err := Content(s, MaxContentBytes); err != nil {
		return err
	}
	if rc := utf8.RuneCountInString(s); rc > MaxContentRunes {
		return fmt.Errorf("%w: %d > %d", ErrTooLongRune, rc, MaxContentRunes)
	}
	return nil
}

// NoteContent 校验笔记内容；放宽到 8 KiB，便于容纳堆栈/错误信息。
func NoteContent(s string) error {
	return Content(s, MaxNoteBytes)
}

// isForbiddenControl 判定是否为禁止的控制字符。
// 允许：\n (0x0A) / \t (0x09)
// 禁止：其他 < 0x20 + DEL (0x7F) + ANSI escape (\x1b 在 < 0x20 集合中)
func isForbiddenControl(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	return r < 0x20 || r == 0x7F
}
