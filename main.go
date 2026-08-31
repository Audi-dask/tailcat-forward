// Command tailcat-forward 把 tailcat 服务端暴露的 TCP 端口映射成本机普通
// TCP 监听端口，让只认 host:port 的工具（浏览器、数据库客户端、IDE 等）
// 无需 SOCKS 或 stdio 支持就能访问远端 tailcat 服务端。
//
// 它不修改 tailcat 本身，只依赖 github.com/tailscale/tailcat 库的
// Client.DialTCPPort 与 ProxyConns 能力，以独立二进制形式运行。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/tailscale/tailcat"
)

func main() {
	bind := flag.String("bind", "127.0.0.1", "监听地址；当映射只给出端口时，用它作为监听 IP")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `用法:
  tailcat-forward [--bind=<addr>] <addrblob> <[local:]remote> [<[local:]remote> ...]

示例:
  tailcat-forward <token> 8080            # 127.0.0.1:8080 -> 远端 8080
  tailcat-forward <token> 18080:8080      # 127.0.0.1:18080 -> 远端 8080
  tailcat-forward --bind=0.0.0.0 <token> 18080:8080   # 允许局域网访问
  tailcat-forward <token> 3306:3306 6379:6379          # 一次映射多个端口
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		flag.Usage()
		os.Exit(2)
	}

	cl := tailcat.NewClient(tailcat.ConnBlob(args[0]))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var listeners []net.Listener
	for _, spec := range args[1:] {
		listenAddr, remotePort, err := parseSpec(*bind, spec)
		if err != nil {
			log.Fatalf("映射 %q 无效: %v", spec, err)
		}
		ln, err := net.Listen("tcp", listenAddr)
		if err != nil {
			log.Fatalf("监听 %s 失败: %v", listenAddr, err)
		}
		listeners = append(listeners, ln)
		go forward(cl, ln, remotePort)
	}

	<-ctx.Done()
	log.Printf("正在退出")
	for _, ln := range listeners {
		ln.Close()
	}
	cl.Close()
}

// forward 接受 ln 上的连接，并把每个连接转发到 tailcat 服务端的 remotePort，
// 复用同一个 client。
func forward(cl *tailcat.Client, ln net.Listener, remotePort uint16) {
	log.Printf("转发 %s -> 远端 localhost:%d", ln.Addr(), remotePort)
	for {
		c, err := ln.Accept()
		if err != nil {
			// 退出时 listener 被关闭，直接返回。
			return
		}
		go func() {
			remote, err := cl.DialTCPPort(context.Background(), remotePort)
			if err != nil {
				log.Printf("拨号远端端口 %d 失败: %v", remotePort, err)
				c.Close()
				return
			}
			// ProxyConns 双向拷贝并传播 TCP 半关闭。
			tailcat.ProxyConns(c, remote)
		}()
	}
}

// parseSpec 把 "[local:]remote" 映射解析成监听地址和远端端口号。
// 单独 "8080" 表示本机 8080 映射到远端 8080；"18080:8080" 表示本机 18080
// 映射到远端 8080。本机 IP 由 --bind 提供。
func parseSpec(bind, spec string) (listenAddr string, remotePort uint16, err error) {
	local, remote, hasColon := strings.Cut(spec, ":")
	if !hasColon {
		remote = local
	}
	rp, err := parsePort(remote)
	if err != nil {
		return "", 0, fmt.Errorf("远端端口: %w", err)
	}
	lp, err := parsePort(local)
	if err != nil {
		return "", 0, fmt.Errorf("本机端口: %w", err)
	}
	return net.JoinHostPort(bind, strconv.Itoa(int(lp))), uint16(rp), nil
}

func parsePort(s string) (uint16, error) {
	p, err := strconv.ParseUint(s, 10, 16)
	if err != nil || p == 0 {
		return 0, fmt.Errorf("无效端口 %q", s)
	}
	return uint16(p), nil
}
