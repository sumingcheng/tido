package validate

import (
	"errors"
	"strings"
	"testing"
)

func TestContent_OK(t *testing.T) {
	cases := []string{
		"hello",
		"中文 + emoji 🚀",
		"line1\nline2\ttab",
		"a", // 1 字节也合法
	}
	for _, s := range cases {
		if err := Content(s, MaxContentBytes); err != nil {
			t.Errorf("Content(%q): unexpected error %v", s, err)
		}
	}
}

func TestContent_Empty(t *testing.T) {
	if err := Content("", 100); !errors.Is(err, ErrEmpty) {
		t.Errorf("Content(\"\"): want ErrEmpty, got %v", err)
	}
}

func TestContent_TooLong(t *testing.T) {
	s := strings.Repeat("a", 101)
	err := Content(s, 100)
	if !errors.Is(err, ErrTooLongByte) {
		t.Errorf("want ErrTooLongByte, got %v", err)
	}
}

func TestContent_NUL(t *testing.T) {
	if err := Content("hello\x00world", 100); !errors.Is(err, ErrContainsNUL) {
		t.Errorf("want ErrContainsNUL, got %v", err)
	}
}

func TestContent_ControlChar(t *testing.T) {
	cases := []string{
		"\x1b[31mred\x1b[0m", // ANSI escape
		"bell\x07",
		"vertical\x0btab",
		"del\x7f",
	}
	for _, s := range cases {
		if err := Content(s, 100); !errors.Is(err, ErrControlChar) {
			t.Errorf("Content(%q): want ErrControlChar, got %v", s, err)
		}
	}
}

func TestContent_AllowsTabAndNewline(t *testing.T) {
	if err := Content("a\tb\nc", 100); err != nil {
		t.Errorf("\\t and \\n must be allowed: %v", err)
	}
}

func TestContent_InvalidUTF8(t *testing.T) {
	bad := string([]byte{0xff, 0xfe, 0xfd})
	if err := Content(bad, 100); !errors.Is(err, ErrNotUTF8) {
		t.Errorf("want ErrNotUTF8, got %v", err)
	}
}

func TestTodoContent_RuneLimit(t *testing.T) {
	// 全部用单字节 ASCII 触发 byte 限制；用 4 字节 emoji 触发 rune 限制。
	s := strings.Repeat("🚀", MaxContentRunes+1) // 每个 4 字节，rune 数 = 2001
	err := TodoContent(s)
	if err == nil {
		t.Fatal("expected error for over-rune string")
	}
	// 可能先撞 byte 上限（4*2001 > 4096），也可能撞 rune 上限——任一即可
	if !errors.Is(err, ErrTooLongByte) && !errors.Is(err, ErrTooLongRune) {
		t.Errorf("want ErrTooLongByte or ErrTooLongRune, got %v", err)
	}
}

func TestScope_OK(t *testing.T) {
	good := []string{"default", "bugfix-2026", "auth.refactor", "a", strings.Repeat("a", 64)}
	for _, s := range good {
		if err := Scope(s); err != nil {
			t.Errorf("Scope(%q): unexpected error %v", s, err)
		}
	}
}

func TestScope_Bad(t *testing.T) {
	bad := []string{"", " ", "has space", "中文", strings.Repeat("a", 65), "a;drop"}
	for _, s := range bad {
		if err := Scope(s); err == nil {
			t.Errorf("Scope(%q): expected error", s)
		}
	}
}

func TestID(t *testing.T) {
	if err := ID("t1"); err != nil {
		t.Errorf("ID(\"t1\"): %v", err)
	}
	if err := ID("invalid"); err == nil {
		t.Error("ID(\"invalid\"): expected error")
	}
}
