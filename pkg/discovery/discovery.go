package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// UDP 发现端口（固定值，Publisher 广播/Viewer 监听）
const discoverPort = 47815

// Magic 标识，避免误解析其他应用的 UDP 包
const magic = "snap-screen-v1"

// Announcement Publisher 广播的数据结构
type Announcement struct {
	Magic string `json:"magic"`
	Name  string `json:"name"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
}

// StartPublisherBeacon 周期性通过 UDP 广播 Publisher 的信令地址。
// host 为局域网可达 IP，port 为信令 HTTP 监听端口。
func StartPublisherBeacon(ctx context.Context, host string, port int) error {
	if host == "" || port <= 0 {
		return fmt.Errorf("invalid host or port for beacon")
	}

	ann := Announcement{
		Magic: magic,
		Name:  host, // 暂用 host 作为名称，后续可替换为自定义名称
		Host:  host,
		Port:  port,
	}
	payload, err := json.Marshal(ann)
	if err != nil {
		return err
	}

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return err
	}
	defer conn.Close()

	broadcastAddr := &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: discoverPort,
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = conn.WriteTo(payload, broadcastAddr)
		}
	}
}

// Discover 在给定超时时间内监听 UDP 广播，收集 Publisher 列表。
func Discover(timeout time.Duration) ([]Announcement, error) {
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", discoverPort))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	var result []Announcement
	seen := make(map[string]bool)

	buf := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			// 超时等错误直接返回已收集的数据
			break
		}
		var ann Announcement
		if err := json.Unmarshal(buf[:n], &ann); err != nil {
			continue
		}
		if ann.Magic != magic || ann.Host == "" || ann.Port <= 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", ann.Host, ann.Port)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, ann)
	}
	return result, nil
}

// LocalIPv4 尝试获取一块非回环的 IPv4 地址，用于对外广播。
func LocalIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		// 跳过未启用/环回网卡
		if iface.Flags&(net.FlagUp|net.FlagLoopback) != net.FlagUp {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			return ip.String()
		}
	}
	return ""
}


