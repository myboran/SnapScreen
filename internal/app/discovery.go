package app

import (
	"encoding/json"
	"log"
	"net"
	"time"
)

// discoveryPort 为局域网内 UDP 广播使用的固定端口
const discoveryPort = 28901

// discoveryPacket 是 Publisher 通过 UDP 广播的内容
type discoveryPacket struct {
	Port int    `json:"port"` // 信令 HTTP 监听端口
	Name string `json:"name"` // 预留字段，当前未在 UI 显示
}

// startDiscoveryBroadcast 在指定端口上通过 UDP 广播 Publisher 存在信息
func startDiscoveryBroadcast(port int) func() {
	if port <= 0 {
		return func() {}
	}

	addr := &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: discoveryPort,
	}

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		log.Println("startDiscoveryBroadcast DialUDP error:", err)
		return func() {}
	}

	stopCh := make(chan struct{})

	go func() {
		defer conn.Close()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		pkt := discoveryPacket{
			Port: port,
			Name: "SnapScreen Publisher",
		}
		data, err := json.Marshal(pkt)
		if err != nil {
			log.Println("marshal discoveryPacket error:", err)
			return
		}

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				if _, err := conn.Write(data); err != nil {
					log.Println("discovery broadcast write error:", err)
				}
			}
		}
	}()

	return func() {
		close(stopCh)
	}
}

// startDiscoveryListener 监听局域网内的 Publisher 广播，并通过回调返回发现的 IP 和端口
func startDiscoveryListener(onFound func(ip string, port int)) {
	if onFound == nil {
		return
	}

	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: discoveryPort,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		log.Println("startDiscoveryListener ListenUDP error:", err)
		return
	}

	go func() {
		defer conn.Close()
		buf := make([]byte, 1024)

		for {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, src, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				log.Println("discovery listener read error:", err)
				continue
			}

			var pkt discoveryPacket
			if err := json.Unmarshal(buf[:n], &pkt); err != nil {
				continue
			}
			if pkt.Port <= 0 {
				continue
			}

			onFound(src.IP.String(), pkt.Port)
		}
	}()
}
