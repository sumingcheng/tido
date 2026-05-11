package validate

import (
	"errors"
	"fmt"
	"regexp"
)

// scopeRE：DESIGN.md §3.3 字符白名单。
var scopeRE = regexp.MustCompile(`^[a-zA-Z0-9_./:-]{1,64}$`)

// ErrInvalidScope 表示 scope 名字符集或长度违规。
var ErrInvalidScope = errors.New("invalid scope")

// Scope 校验 scope 名（^[a-zA-Z0-9_./:-]{1,64}$）。
func Scope(s string) error {
	if !scopeRE.MatchString(s) {
		return fmt.Errorf("%w: %q (must match ^[a-zA-Z0-9_./:-]{1,64}$)", ErrInvalidScope, s)
	}
	return nil
}
