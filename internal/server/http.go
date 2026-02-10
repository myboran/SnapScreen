package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"
)

// StartHTTPServer 在当前进程内启动一个使用 WebSocket 信令的 HTTP 服务器。
// addr 形如 ":8080" 或 "127.0.0.1:0"（端口为 0 时由系统自动分配）。
// 返回实际监听地址、用于优雅关闭的 stop 函数，以及错误信息。
func StartHTTPServer(addr string) (string, func(), error) {
	s := NewServer()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.ServeWS)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, err
	}

	actualAddr := ln.Addr().String()

	srv := &http.Server{
		Handler: mux,
	}

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("embedded signaling server error: %v", err)
		}
	}()

	stop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("embedded signaling server shutdown error: %v", err)
		}
	}

	return actualAddr, stop, nil
}
