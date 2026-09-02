[English](./README_EN.md) | 简体中文

# tailcat-forward

把 [tailcat](https://github.com/tailscale/tailcat) 服务端暴露的 TCP 端口，映射成本机普通的 TCP 监听端口。

让只认 `host:port` 的工具——浏览器、数据库客户端（MySQL/Redis/PostgreSQL）、IDE、脚本等——无需 SOCKS 或 stdio 支持，就能访问远端 tailcat 服务端。

底层基于 `github.com/tailscale/tailcat` 库，不改动 tailcat 本身。

## 安装

### 下载 Release 二进制

在 [Releases](https://github.com/Audi-dask/tailcat-forward/releases) 页面下载对应平台的压缩包（linux / darwin / windows × amd64 / arm64）。

### 从源码编译

```bash
go install github.com/Audi-dask/tailcat-forward@latest
```

## 快速开始

### 1. 服务端暴露端口

在远端服务器上，用 tailcat 暴露本地端口：

```bash
tailcat serve 8080
# 🐈 Server listening with new address: tcXXXXXXXXXXXXXXXX...
```

复制输出的 `tc...` token。

### 2. 本地映射端口

```bash
tailcat-forward tcXXXXXXXXXXXXXXXX... 18080:8080
# 转发 127.0.0.1:18080 -> 远端 localhost:8080
```

### 3. 用普通工具访问本地端口

```bash
curl http://127.0.0.1:18080/
```

浏览器直接打开 `http://127.0.0.1:18080/` 即可。

## 用法

```text
tailcat-forward [--bind=<addr>] <addrblob> <[local:]remote> [<[local:]remote> ...]
```

| 示例 | 效果 |
|---|---|
| `tailcat-forward <token> 8080` | 本机 `127.0.0.1:8080` → 远端 `8080` |
| `tailcat-forward <token> 18080:8080` | 本机 `127.0.0.1:18080` → 远端 `8080` |
| `tailcat-forward --bind=0.0.0.0 <token> 18080:8080` | 局域网内其他设备也能访问 |
| `tailcat-forward <token> 3306:3306 6379:6379` | 一次映射 MySQL + Redis |


## 示例

把远端 Jenkins 后台映射到本机：

```bash
tailcat-forward <token> 18080:8080
```

然后浏览器打开 `http://127.0.0.1:18080/`。

把远端 MySQL、Redis 一次映射出来：

```bash
tailcat-forward <token> 3306:3306 6379:6379
mysql -h 127.0.0.1 -P 3306 -u root -p
redis-cli -h 127.0.0.1 -p 6379
```

## 说明

- 每个连接都复用同一个 tailcat 客户端，避免重复握手与 NAT 穿透。
- 通过 `ProxyConns` 双向拷贝并传播 TCP 半关闭，支持需要请求体结束才响应的协议。
- `Ctrl-C` 优雅退出。
