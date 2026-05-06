package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// IMAPReader 在握手阶段按行读取 IMAP 帧，处理 literal {N} 语法。
// 握手完成后调用者切换到原始 io.Copy，不再使用本结构。
type IMAPReader struct {
	r *bufio.Reader
}

func newIMAPReader(r io.Reader) *IMAPReader {
	return &IMAPReader{r: bufio.NewReader(r)}
}

// ReadLine 读取一个完整的 IMAP 帧（含 literal 内容），返回拼接后的完整字节。
// 对纯行命令返回 "<line>\r\n"；对含 literal 的命令返回 "<line>\r\n<N bytes>"。
func (ir *IMAPReader) ReadLine() ([]byte, error) {
	var result []byte
	for {
		line, err := ir.r.ReadString('\n')
		result = append(result, []byte(line)...)
		if err != nil {
			return result, err
		}
		// 检查行末是否有 literal 计数 {N}
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.HasSuffix(trimmed, "}") {
			open := strings.LastIndex(trimmed, "{")
			if open >= 0 {
				nStr := trimmed[open+1 : len(trimmed)-1]
				n, convErr := strconv.Atoi(nStr)
				if convErr == nil && n >= 0 {
					lit := make([]byte, n)
					if _, readErr := io.ReadFull(ir.r, lit); readErr != nil {
						return result, readErr
					}
					result = append(result, lit...)
					continue // literal 后面还有下一行
				}
			}
		}
		return result, nil
	}
}

// Buffered 返回 bufio.Reader 中已经读入但未消费的数据，用于在切换到 io.Copy 时回放。
func (ir *IMAPReader) Buffered() []byte {
	n := ir.r.Buffered()
	if n == 0 {
		return nil
	}
	buf, _ := ir.r.Peek(n)
	out := make([]byte, n)
	copy(out, buf)
	ir.r.Discard(n)
	return out
}

// ParseTag 从一行 IMAP 帧里提取 tag 和命令名（或响应类型）。
// 格式：<tag> <cmd/status> [args...]
// 未标记的行（以 * 或 + 开头）tag 返回 "*" 或 "+"。
func ParseTag(line string) (tag, cmd string) {
	line = strings.TrimRight(line, "\r\n")
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		return "", ""
	}
	return parts[0], strings.ToUpper(parts[1])
}

// BuildIDCommand 根据配置构建 ID 命令字符串。
func BuildIDCommand(idTag string, id IMAPIDConfig) string {
	return fmt.Sprintf(
		"%s ID (\"name\" %q \"version\" %q \"vendor\" %q \"support-email\" %q)\r\n",
		idTag, id.Name, id.Version, id.Vendor, id.SupportEmail,
	)
}
