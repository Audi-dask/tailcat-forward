# 自建 DERP（Manual 自签 IP 证书版）+ tailcat-forward 使用说明

## 架构

```
浏览器 → 127.0.0.1:18080
              ↓
         tailcat-forward（本地）
              ↓ WireGuard 加密隧道
         自建 DERP（<your-derp-ip>:8003）
              ↓
         tailcat serve（远端服务器）
              ↓
         目标服务（如 Jenkins 8080）
```

## 前置条件

- 已部署自建 DERP（本文以 **manual 自签 IP 证书**模式为例）
- 已托管 DERPMap JSON 到 HTTPS 地址
- 服务端和本地均已安装 tailcat + tailcat-forward

## 第一步：准备 DERPMap JSON

创建 `derpmap.json`，填入你的 DERP 信息：

```json
{
  "Regions": {
    "915": {
      "RegionID": 915,
      "RegionCode": "MY_DERP",
      "RegionName": "My Private Derp",
      "Nodes": [{
        "Name": "915",
        "RegionID": 915,
        "HostName": "<your-derp-ip>",
        "IPv4": "<your-derp-ip>",
        "DERPPort": 8003,
        "STUNPort": 3478,
        "CertName": "sha256-raw:<your-cert-fingerprint>"
      }]
    }
  }
}
```

字段说明：

| 字段 | 值 | 说明 |
|---|---|---|
| `RegionID` | 915 | 自定义编号，不能与官方 region 冲突 |
| `HostName` | `<your-derp-ip>` | 不能用域名（manual 模式按 IP 签证书） |
| `DERPPort` | 8003 | DERP 容器映射的宿主机端口 |
| `STUNPort` | 3478 | STUN UDP 端口 |
| `CertName` | `sha256-raw:xxx` | 从 `docker logs derp` 获取，自签证书指纹 |

上传到 HTTPS 地址，例如：

```
https://<your-domain>/derpmap.json
```

## 第二步：服务端配置

### 2.1 生成固定密钥（绑定 region 915）

```bash
./tailcat --derpmap-url='https://<your-domain>/derpmap.json' genkey --key=default --region=915
```

密钥写入 `/root/.config/tailcat/keys/default.private.json`，token 重启不变。

### 2.2 启动服务（使用 --full-address 输出自包含 token）

```bash
./tailcat --derpmap-url='https://<your-domain>/derpmap.json' --full-address --serve=8080
```

输出：

```
# Selected bootstrap relay region 915, My Private Derp
# 🐈 Server listening with saved key "default": tco2FwWCDc9kxLTWF-g7biwCCWaGGs6...
```

`--full-address` 的作用：把 DERP 的地址、端口、证书指纹全部内嵌进 token，客户端拿到 token 即可连接，**不需要传 --derpmap-url**。

### 2.3 genkey 与 --full-address 的 token 关系

`genkey` 和 `serve --full-address` 使用**同一个持久化私钥**（`default.private.json`），但输出的 token 字符串不同：

| 命令 | token 内容 | 长度 |
|---|---|---|
| `genkey` | 私钥 + RegionID 引用 | 短（~90 字符） |
| `serve`（无 `--full-address`） | 同上 | 短 |
| `serve --full-address` | 私钥 + DERP 完整详情（IP/端口/证书） | 长（~300+ 字符） |

**固定密钥完全可用**：`--full-address` 只是在同一个私钥基础上多塞了 DERP 信息，不重新生成密钥。重启服务端只要不删 `default.private.json`，密钥不变，`--full-address` 输出的长 token 也不变。

### 2.4 --full-address vs 不带 --full-address

| | `--full-address` | 不带 `--full-address` |
|---|---|---|
| token 长度 | 长（~300+ 字符，含 DERP 详情） | 短（~90 字符，只含 region 引用） |
| 客户端需要 `--derpmap-url` | 不需要 | 需要 |
| tailcat-forward 兼容 | 直接可用 | 需要额外加参数 |
| DERP 配置是否暴露 | 暴露（token 里有 IP+证书） | 不暴露 |

**推荐用 `--full-address`**：tailcat-forward 目前不支持 `--derpmap-url` 参数，自包含 token 是最短路径。

### 2.5 避免终端复制截断 token

```bash
TAILCAT_ADDR_FILE=/tmp/token.txt ./tailcat --derpmap-url='https://<your-domain>/derpmap.json' --full-address --serve=8080 &
sleep 2
cat /tmp/token.txt   # 完整 token，无截断
```

## 第三步：客户端配置

### 3.1 测试延迟

```bash
tailcat ping '<完整token>'
```

期望输出：

```
pong in 248ms via DERP(MY_DERP)
```

`via DERP` = 流量走你的自建中继。如果看到 `via IP:port` 说明 P2P 直连成功（DERP 仅做信令兜底）。

### 3.2 启动 tailcat-forward 端口映射

```bash
tailcat-forward '<完整token>' 18080:8080
```

输出：

```
转发 127.0.0.1:18080 -> 远端 localhost:8080
```

### 3.3 访问

浏览器打开：

```
http://127.0.0.1:18080/
```

或命令行：

```bash
curl http://127.0.0.1:18080/
```

### 3.4 多端口映射

```bash
tailcat-forward '<token>' 3306:3306 6379:6379 18080:8080
```

### 3.5 允许局域网其他设备访问

```bash
tailcat-forward --bind=0.0.0.0 '<token>' 18080:8080
```

局域网其他设备访问 `http://<本机IP>:18080/`。

## 注意事项

1. **token 是持久化的**：用 `genkey --key=default` 生成的密钥长期有效，进程重启 token 不变。不要泄露完整 token。
2. **DERP 容器卷勿删**：`derp-data` 卷保存证书和节点私钥，删除后 `CertName` 会变，需要同步更新 derpmap.json。
3. **P2P 直连不一定成功**：两端都在 NAT/防火墙后时，流量全走 DERP 中继。DERP 选点应靠近客户端。
4. **旧版 tailcat CLI 差异**：v0.3.0 用 `--serve=8080`（flag），main 分支用 `serve 8080`（子命令）。两者 token 兼容。
