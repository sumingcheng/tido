package validate

import (
	"errors"
	"fmt"

	"github.com/sumingcheng/tido/internal/shortid"
)

// ErrInvalidID 表示短码格式不合规。
var ErrInvalidID = errors.New("invalid id")

// ID 校验短码格式（^t[0-9a-z]{1,8}$）。
func ID(s string) error {
	if !shortid.Valid(s) {
		return fmt.Errorf("%w: %q", ErrInvalidID, s)
	}
	return nil
}
