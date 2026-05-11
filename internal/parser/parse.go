package parser

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Item 是 batch 内单个任务的中间表示。
// 应用层据此构造 store 层的 Todo（补充 scope / parent_id / priority 等顶层入参）。
type Item struct {
	Content string // 任务内容（已 trim）
	Status  string // pending / in_progress / completed / cancelled
	Depth   int    // 缩进层级，0 = 顶层；缩进每 2 空格 +1
}

// Format 表示输入的格式类型。
type Format int

const (
	FormatUnknown Format = iota
	FormatMarkdown
	FormatText
)

// markdownLineRE 解析单行 checkbox：捕获 (缩进, 状态字符, 内容)。
var markdownLineRE = regexp.MustCompile(`^( *)-\s+\[([ x\-~])\]\s+(.+)$`)

// markdownDetectRE 用于"看起来像 markdown checklist"的探测：任一非空行匹配即认。
var markdownDetectRE = regexp.MustCompile(`(?m)^\s*-\s+\[[ x\-~]\]\s+`)

// ErrParse 表示解析失败；调用方可用 errors.Is 检查类别。
var ErrParse = errors.New("parse error")

// Detect 探测格式：含任一 markdown checkbox 行 → Markdown，否则 → Text，全空 → Unknown。
func Detect(s string) Format {
	s = Normalize(s)
	if s == "" {
		return FormatUnknown
	}
	if markdownDetectRE.MatchString(s) {
		return FormatMarkdown
	}
	return FormatText
}

// Parse 自动识别格式并归一化为 []Item。
// 任一行解析失败即整体失败（fail fast，避免静默丢任务）。
func Parse(s string) ([]Item, error) {
	s = Normalize(s)
	if s == "" {
		return nil, fmt.Errorf("%w: input is empty", ErrParse)
	}
	switch Detect(s) {
	case FormatMarkdown:
		return parseMarkdown(s)
	case FormatText:
		return parseText(s)
	default:
		return nil, fmt.Errorf("%w: cannot detect format", ErrParse)
	}
}

// parseMarkdown 解析 markdown checklist。
// 非 checkbox 行（标题、空行、笔记等）静默忽略——markdown 模式宽容。
func parseMarkdown(s string) ([]Item, error) {
	var items []Item
	for i, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := markdownLineRE.FindStringSubmatch(line)
		if m == nil {
			continue // 非任务行（标题、文本等），跳过
		}
		indent := len(m[1])
		if indent%2 != 0 {
			return nil, fmt.Errorf("%w: line %d indent must be even (got %d): %q", ErrParse, i+1, indent, line)
		}
		status, ok := markerToStatus(m[2][0])
		if !ok {
			return nil, fmt.Errorf("%w: line %d unknown marker %q", ErrParse, i+1, m[2])
		}
		items = append(items, Item{
			Content: strings.TrimSpace(m[3]),
			Status:  status,
			Depth:   indent / 2,
		})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: no checkbox lines found in markdown", ErrParse)
	}
	return items, nil
}

// parseText 把每一非空行当作一个 pending todo（无父子）。
func parseText(s string) ([]Item, error) {
	var items []Item
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		items = append(items, Item{Content: t, Status: "pending", Depth: 0})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: no non-empty lines", ErrParse)
	}
	return items, nil
}

// markerToStatus 把 markdown checkbox 字符映射到 status 枚举。
// DESIGN.md §5 表：' '→pending、'x'→completed、'-'→in_progress、'~'→cancelled。
func markerToStatus(c byte) (string, bool) {
	switch c {
	case ' ':
		return "pending", true
	case 'x':
		return "completed", true
	case '-':
		return "in_progress", true
	case '~':
		return "cancelled", true
	}
	return "", false
}
