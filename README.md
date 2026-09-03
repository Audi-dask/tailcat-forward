[English](./README_EN.md) | 简体中文

# tailcat-forward

> **归档准备中：这是一个基于官方 `tailcat` 的功能验证仓库。**

在官方 [tailscale/tailcat](https://github.com/tailscale/tailcat) 尚未支持客户端 TCP 转发时，我基于其代码实现并验证了 TCP 端口转发能力，使只支持普通 `host:port` 连接的浏览器、数据库客户端、IDE 和脚本能够访问远程 Tailcat 服务。

相关实现已经提交到上游：

- ✅ [PR #62](https://github.com/tailscale/tailcat/pull/62) — 增加 `tailcat forward`，已合并
- 🚧 [PR #75](https://github.com/tailscale/tailcat/pull/75) — 支持转发到 Exit Node 目标，进行中

本仓库保留作为功能验证和实现过程记录。后续请优先关注官方上游项目：

👉 [tailscale/tailcat](https://github.com/tailscale/tailcat)

> 本仓库不再作为独立项目持续维护。下面的安装和使用内容保留用于复现当时的验证过程。

## 历史验证：安装

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
