package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
)

func handleConn(clientConn net.Conn, cfg Config) {
	defer clientConn.Close()

	remote := clientConn.RemoteAddr().String()
	slog.Info("client connected", "remote", remote)

	upstreamConn, err := tls.Dial("tcp", cfg.Upstream, &tls.Config{
		ServerName: cfg.UpstreamTLSName,
	})
	if err != nil {
		slog.Error("upstream connect failed", "err", err)
		return
	}
	defer upstreamConn.Close()

	if err := runHandshake(clientConn, upstreamConn, cfg); err != nil {
		if err != io.EOF && !strings.Contains(err.Error(), "use of closed") {
			slog.Error("handshake error", "remote", remote, "err", err)
		}
		return
	}

	slog.Info("entering transparent relay", "remote", remote)
	relay(clientConn, upstreamConn)
}

func runHandshake(client net.Conn, upstream *tls.Conn, cfg Config) error {
	clientReader := newIMAPReader(client)
	serverReader := newIMAPReader(upstream)

	// loginTag 由 client→server goroutine 写入，server→client goroutine 读取
	var loginTag atomic.Value // stores string

	// server→client goroutine 完成 ID 注入后把 LOGIN OK 行发到这里
	holdDone := make(chan []byte, 1)
	// 任意一侧出错时通过此 channel 上报
	errCh := make(chan error, 2)

	// server → client：持续转发，直到看到 loginTag 对应的 OK
	go func() {
		for {
			line, err := serverReader.ReadLine()
			if err != nil {
				errCh <- err
				return
			}
			slog.Debug("server→client", "line", strings.TrimRight(string(line), "\r\n"))

			tag, status := ParseTag(string(line))

			lt, _ := loginTag.Load().(string)
			if lt != "" && strings.EqualFold(tag, lt) && status == "OK" {
				slog.Debug("server: LOGIN OK received, injecting ID", "tag", lt)

				idTag := "_proxy_id_1"
				idCmd := BuildIDCommand(idTag, cfg.IMAPID)
				if _, wErr := fmt.Fprint(upstream, idCmd); wErr != nil {
					errCh <- wErr
					return
				}

				// 读取 ID 响应直到 idTag OK/NO/BAD
				for {
					idLine, rErr := serverReader.ReadLine()
					if rErr != nil {
						errCh <- rErr
						return
					}
					t, s := ParseTag(string(idLine))
					slog.Debug("server: ID response", "line", strings.TrimRight(string(idLine), "\r\n"))
					if strings.EqualFold(t, idTag) {
						if s != "OK" {
							slog.Warn("ID command returned non-OK", "status", s)
						}
						break
					}
				}

				slog.Info("ID injected successfully", "tag", lt)
				holdDone <- line // 把 LOGIN OK 交给主 goroutine 转发
				return
			}

			// 普通帧直接转发
			if _, wErr := client.Write(line); wErr != nil {
				errCh <- wErr
				return
			}
		}
	}()

	// client → server：转发并识别 LOGIN/AUTHENTICATE
	for {
		line, err := clientReader.ReadLine()
		if err != nil {
			return err
		}
		slog.Debug("client→server", "line", RedactSensitive(string(line)))

		if _, wErr := upstream.Write(line); wErr != nil {
			return wErr
		}

		// 还没检测到 LOGIN 的情况下才解析
		if lt, _ := loginTag.Load().(string); lt == "" {
			tag, cmd := ParseTag(string(line))
			if cmd == "LOGIN" || cmd == "AUTHENTICATE" {
				slog.Debug("detected LOGIN command", "tag", tag)
				loginTag.Store(tag)

				// 等待 server goroutine 完成 ID 注入
				select {
				case buf := <-holdDone:
					if _, wErr := client.Write(buf); wErr != nil {
						return wErr
					}
				case err := <-errCh:
					return err
				}

				// 回放两侧 reader 里已缓冲但未消费的字节
				if cb := clientReader.Buffered(); len(cb) > 0 {
					if _, wErr := upstream.Write(cb); wErr != nil {
						return wErr
					}
				}
				if sb := serverReader.Buffered(); len(sb) > 0 {
					if _, wErr := client.Write(sb); wErr != nil {
						return wErr
					}
				}

				return nil // 握手完成，进入 relay
			}
		}
	}
}

func relay(client net.Conn, upstream *tls.Conn) {
	var wg sync.WaitGroup
	cp := func(dst io.Writer, src io.Reader) {
		defer wg.Done()
		io.Copy(dst, src)
	}
	wg.Add(2)
	go cp(upstream, client)
	go cp(client, upstream)
	wg.Wait()
}
