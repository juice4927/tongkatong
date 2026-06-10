package utils

import (
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

// CheckNetworkConnectivity 检查网络连通性
func CheckNetworkConnectivity(probes []struct{ Host string; Port int }, timeout time.Duration) bool {
	targets := probes
	if len(targets) == 0 {
		targets = DefaultProbeTargets
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	for _, target := range targets {
		addr := net.JoinHostPort(target.Host, itoa(target.Port))
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err == nil {
			conn.Close()
			return true
		}
		slog.Debug("网络探测失败", "target", addr, "error", err)
	}

	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
