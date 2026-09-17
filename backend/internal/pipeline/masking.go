package pipeline

import (
	"bytes"
	"io"
)

// maskingWriter 在写入日志前把已注入的 secret 值替换为 ***，避免步骤输出泄露密钥。
type maskingWriter struct {
	w       io.Writer
	secrets [][]byte
}

// newMaskingWriter 用给定 secret 值构造掩码写入器；无有效值时原样返回 w。
// 过短（<4 字节）的值不参与掩码，避免误伤正常日志。
func newMaskingWriter(w io.Writer, values []string) io.Writer {
	m := &maskingWriter{w: w}
	for _, v := range values {
		if len(v) >= 4 {
			m.secrets = append(m.secrets, []byte(v))
		}
	}
	if len(m.secrets) == 0 {
		return w
	}
	return m
}

func (m *maskingWriter) Write(p []byte) (int, error) {
	out := p
	for _, s := range m.secrets {
		if bytes.Contains(out, s) {
			out = bytes.ReplaceAll(out, s, []byte("***"))
		}
	}
	if _, err := m.w.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}
