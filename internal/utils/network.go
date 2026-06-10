package utils

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"
)

// DefaultProbeTargets 默认网络探测目标
var DefaultProbeTargets = []struct {
	Host string
	Port int
}{
	{"223.5.5.5", 53},
	{"114.114.114.114", 53},
	{"1.1.1.1", 53},
	{"8.8.8.8", 53},
}

// CheckNetworkConnectivity 并行检查网络连通性（任一目标可达即返回 true，总超时 = timeout）
func CheckNetworkConnectivity(probes []struct{ Host string; Port int }, timeout time.Duration) bool {
	targets := probes
	if len(targets) == 0 {
		targets = DefaultProbeTargets
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultCh := make(chan bool, len(targets))
	for _, target := range targets {
		go func(host string, port int) {
			addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
			conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
			if err == nil {
				conn.Close()
				resultCh <- true
			} else {
				resultCh <- false
			}
		}(target.Host, target.Port)
	}

	for i := 0; i < len(targets); i++ {
		select {
		case ok := <-resultCh:
			if ok {
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
	return false
}
