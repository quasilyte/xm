package xmfile

import (
	"bytes"
)

func convertCstring(data []byte) string {
	i := bytes.IndexByte(data, 0)
	if i == -1 {
		return string(data)
	}
	return string(data[:i])
}
