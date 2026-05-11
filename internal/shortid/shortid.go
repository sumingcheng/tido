// Package shortid 提供 base36 短码编解码：rowid → "t" + base36(n)。
// 设计依据：DESIGN.md §3.3 id 字段说明。
package shortid

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Prefix 是所有短码的固定前缀。
const Prefix = "t"

// 短码格式：t + 1~8 位 base36（约 2.8 万亿 id 空间）。
var validRE = regexp.MustCompile(`^t[0-9a-z]{1,8}$`)

// ErrInvalid 表示字符串不是合法短码。
var ErrInvalid = errors.New("invalid shortid")

// Encode 把正整数 n 编码为短码（如 1 → "t1", 118 → "t3a"）。
// n ≤ 0 视为 bug，直接 panic（生成器内部使用，不应越界）。
func Encode(n int64) string {
	if n <= 0 {
		panic(fmt.Sprintf("shortid.Encode: n must be > 0, got %d", n))
	}
	return Prefix + strconv.FormatInt(n, 36)
}

// Decode 把短码反解为整数（"t1" → 1, "t3a" → 118）。
func Decode(s string) (int64, error) {
	if !Valid(s) {
		return 0, fmt.Errorf("%w: %q", ErrInvalid, s)
	}
	return strconv.ParseInt(s[len(Prefix):], 36, 64)
}

// Valid 校验字符串格式（不解码）。
func Valid(s string) bool {
	return validRE.MatchString(s)
}
