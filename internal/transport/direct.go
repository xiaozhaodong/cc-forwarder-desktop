package transport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CreateSourcePortTransport 创建使用专用源端口范围的传输层。
// 配合 Surge 的 SRC-PORT,<start>-<end>,DIRECT 规则，可以只让调用方选中的请求直连，
// 不影响同进程、同域名的其他请求。调用方应在请求结束后关闭空闲连接。
func CreateSourcePortTransport(startPort, endPort int) (*http.Transport, error) {
	if err := validateSourcePortRange(startPort, endPort); err != nil {
		return nil, err
	}
	transport := newBaseTransport()
	transport.Proxy = nil
	transport.DialContext = sourcePortDialContext(startPort, endPort)

	slog.Info("图像生成专用源端口传输已创建",
		"source_port_start", startPort,
		"source_port_end", endPort)
	return transport, nil
}

// ParseSourcePortRange 解析单个端口或 start-end 形式的专用源端口范围。
func ParseSourcePortRange(raw string) (int, int, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, "-")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, 0, fmt.Errorf("生图直连源端口范围格式必须为单个端口或 start-end，例如 31080-31179")
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("生图直连源端口范围格式必须为单个端口或 start-end，例如 31080-31179")
	}
	end := start
	if len(parts) == 2 {
		end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("生图直连源端口范围格式必须为单个端口或 start-end，例如 31080-31179")
		}
	}
	if err := validateSourcePortRange(start, end); err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func validateSourcePortRange(startPort, endPort int) error {
	if startPort < 1024 || endPort > 65535 || startPort > endPort {
		return fmt.Errorf("生图直连源端口范围必须位于 1024-65535，且起始端口不能大于结束端口")
	}
	if endPort-startPort > 255 {
		return fmt.Errorf("生图直连源端口范围最多包含 256 个端口")
	}
	return nil
}

func sourcePortDialContext(startPort, endPort int) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		var lastErr error
		for port := startPort; port <= endPort; port++ {
			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				LocalAddr: &net.TCPAddr{Port: port},
			}
			connection, err := dialer.DialContext(ctx, network, address)
			if err == nil {
				return connection, nil
			}
			lastErr = err
			if !errors.Is(err, syscall.EADDRINUSE) {
				return nil, err
			}
		}
		return nil, fmt.Errorf("生图直连源端口范围 %d-%d 当前均不可用: %w", startPort, endPort, lastErr)
	}
}
