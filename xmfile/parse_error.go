package xmfile

import (
	"fmt"
)

type ParseError struct {
	Message string

	Offset int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s (offset=%d)", e.Message, e.Offset)
}
