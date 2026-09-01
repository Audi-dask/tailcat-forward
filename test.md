# 基于 Tailcat 的临时特权访问系统 —— 设计文档

> 版本：v1.0（最终版）
> 定位：内部审批制临时访问网关（PAM 轻量实现），飞书登录 + 审批流 + tailcat 作执行层

---

## 1. 项目概述

用户通过飞书登录，选择区域/资产，提交访问申请，经资产技术负责人审批后，系统自动在对应区域网关上开通一条基于 tailcat 的加密隧道，用户本地转发到生产资产，会话限时有效，到期后自动回收。

核心价值：把"谁能访问什么资产"的审批留痕做在前面，把"临时开通、限时收回"的执行动作自动化，避免长期开放的堡垒机账号/固定端口暴露。

---

## 2. 整体流程

```
用户飞书登录 → 选择区域 → 带出区域网关与资产 → 选择资产+端口 → 提交申请
  → 审批流（通知资产技术负责人）
    → 拒绝：结束，通知申请人
    → 通过：触发 agent 创建会话
        → agent 生成客户端密钥对 + 创建独立 Server 实例
        → 面板展示连接方式（token + client-key + 命令 + 下载地址）
        → 用户本地执行 tailcat-forward 建立转发
        → 本地访问生产资产
  → TTL 到期 → agent 调 Server.Close() → token 失效 + 连接断开 + 无法重连
  → 用户可在到期前主动吊销（同样调 Close()）
```

---

## 3. 需求清单

### 3.1 审批与触发时机
- **先审批，后拉起**：审批通过才创建 Server 实例和生成凭证，不允许审批前预先创建。
- 暂不做多级审批，单人审批即可（资产技术负责人）。
- 审批通过后的开通动作若失败（网关不可达、执行异常等），需要有明确的失败终态并通知申请人。

### 3.2 并发与隔离
- 不存在"端口冲突"概念。每个会话有独立的客户端密钥（nodekey）和独立的 Server 实例，互不影响。
- 同资产同端口被多人申请时，各自独立审批、独立创建会话，互不干扰。

### 3.3 网关执行模型
- 每个区域 LAN 部署一个 agent（Go 单二进制），内嵌 tailcat Go 库。
- Agent 以 `OnTCPForward` 方式将客户端流量转发到 LAN 内实际资产 IP:port。
- 客户端不需要知道内网 IP 拓扑，只看到端口号。

### 3.4 凭证与会话有效期
- 会话固定有效期（当前设计为 1 小时），由 `time.AfterFunc` 自动回收。
- 用户可以在到期前在站内主动吊销会话。
- 吊销/到期后：`Server.Close()` 关闭 WireGuard 设备 → token 失效 → 连接断开 → 用户无法重连。无需客户端轮询。

### 3.5 审计
- 记录字段：发起时间、目标资产、持续时长、操作人。
- 不记录流量内容本身（技术上也无法记录，隧道是端到端加密）。

### 3.6 站内交互
- 面板展示：连接凭证（token）、客户端私钥、连接命令、tailcat-forward 下载地址。
- 用户主动吊销入口在会话详情/列表页提供。

---

## 4. 关键技术事实（基于 tailcat v0.3.0 源码验证）

### 4.1 身份与密钥

| 事实 | 代码位置 |
|---|---|
| 一个 Server = 一个 WireGuard 身份 + 一个 token | [tailcat.go#L315-L388](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L315-L388) |
| 密钥分临时和保存两种，临时密钥进程退出即失效 | [tailcat.go#L390-L423](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L390-L423) |
| `NewPrivateKey()` 可纯内存生成密钥对，不落盘 | [tailcat.go#L222-L239](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L222-L239) |
| 公钥通过 `priv.Private.Public()` 获取，直接用于白名单 | 同上 |

### 4.2 访问控制

| 事实 | 代码位置 |
|---|---|
| `AddAllowedClient(k)` 支持运行时动态加白名单，线程安全 | [tailcat.go#L678-L691](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L678-L691) |
| **不存在 `RemoveAllowedClient`**——客户端一旦加入无法移除 | 全仓库零搜索结果 |
| 白名单校验只阻止新握手，不影响已建立连接 | [tailcat.go#L1347-L1362](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L1347-L1362) |
| CLI `--allow` 只在启动时解析一次，不支持热更新 | [cmd/tailcat/tailcat.go#L95-L97](file:///Users/mac/Documents/lokiv3/tailcat/cmd/tailcat/tailcat.go#L95-L97) |

**结论：必须走 Go 库嵌入方式，不能走 CLI 常驻 + 热更新。**

### 4.3 Server 生命周期

| 事实 | 代码位置 |
|---|---|
| `Server.Start()` 初始化 WireGuard + netstack + DERP 连接 | [tailcat.go#L390-L539](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L390-L539) |
| `Server.Close()` 关闭 WireGuard 设备，底层 UDP 全断 | [tailcat.go](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go) |
| `Server.ConnBlob()` 返回自包含 token（内嵌 DERP 信息） | [tailcat.go#L693-L734](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L693-L734) |
| 多个 Server 实例可在同一进程运行，各自独立 WireGuard 设备 | netstack 用户态 TCP 栈，不冲突 |

### 4.4 端口转发

| 事实 | 说明 |
|---|---|
| `ServedTCPPorts` 限制 Server 接受哪些端口的连接 | [tailcat.go#L531-L539](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L531-L539) |
| `OnTCPForward` 回调决定连接转发到哪个实际目标 | Server 结构体字段 |
| 服务端可设定转发规则，客户端只看到端口号 | 客户端不需要知道内网 IP |

### 4.5 DERP 依赖

| 事实 | 代码位置 |
|---|---|
| DERP 是必需的（bootstrap rendezvous），不能去掉 | [tailcat.go#L278-L279](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L278-L279) |
| 可自建 DERP，不依赖官方公共中继 | [tailcat.go#L24-L25](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L24-L25) |
| `ConnBlob()` 默认内嵌 DERP 详情，客户端无需额外配置 | [tailcat.go#L693-L734](file:///Users/mac/Documents/lokiv3/tailcat/tailcat.go#L693-L734) |
| 自建 DERP 部署指南见 [DERP.md](file:///Users/mac/Documents/lokiv3/tailcat-forward/DERP.md) | — |

### 4.6 官方定位

> "Tailcat intentionally leaves out accounts, identity, policies, device management, and persistent network access."

本系统要做的审批 + 身份 + 策略层，正是 tailcat 官方认为需要上层系统补齐的部分。

---

## 5. 最终架构

### 5.1 核心决策：每会话独立 Server 实例

需求为"TTL 到期后阻止重连"，不需要"强制踢人"。因此采用**每会话一个独立 Server 实例**：

```
审批通过
  ↓
agent 生成客户端密钥对（NewPrivateKey，纯内存不落盘）
  ↓
agent 创建新 Server 实例
  ├── Key = 网关固定密钥（持久化）
  ├── DERPMapURL = 自建 DERP map
  ├── ServedTCPPorts = [目标端口]
  ├── AllowedClients = [本次客户端公钥]
  └── OnTCPForward = 转发到实际资产 IP:port
  ↓
Server.Start() → ConnBlob() 获取 token
  ↓
返回 token + 客户端私钥 给用户
  ↓
用户本地 tailcat-forward --client-key='<私钥>' '<token>' 13306:3306
  ↓
TTL 到期 → Server.Close()
  ↓
token 失效 + 连接断开 + 无法重连（自动发生，无需轮询）
```

**为什么可行：**
- tailcat 用 netstack（用户态 TCP 栈），多个 Server 实例可同时 serve 相同端口，不冲突
- `Server.Close()` 关闭 WireGuard 设备，底层 UDP 全断，`DialTCPPort` 返回错误
- token 随 Server 关闭即作废，重连的 meow 无处可去
- 其他会话的 Server 实例完全不受影响
- **不需要 `RemoveAllowedClient`**——整个 Server 关了，白名单全清

**资源开销：**
- 每实例 ~30MB 内存 + 一个 WireGuard engine + 一条 DERP 连接
- DERP 只做 bootstrap，P2P 建立后不跑流量
- 10 并发会话 ≈ 300MB，小团队无压力

### 5.2 服务端转发模型

Agent 通过 `OnTCPForward` 在服务端决定流量去哪，客户端只看到端口号：

```
用户 tailcat-forward --client-key='...' '<token>' 13306:3306
                                                     ↑ 只有序号
    ↓
agent Server 实例（端口 3306）
    ↓ OnTCPForward: 3306 → 192.168.1.10:3306（agent 自己决定）
    ↓
LAN 内实际资产 192.168.1.10:3306
```

**优势：**
- 客户端不知道内网 IP 拓扑
- 服务端控制每个会话只能连到审批指定的资产
- 多资产映射：一个会话配多个端口转发规则，客户端一个命令带多端口

### 5.3 部署拓扑

```
LAN A（墨西哥机房）              LAN B（上海机房）
  ┌────────────────┐             ┌────────────────┐
  │ agent-A        │             │ agent-B        │
  │ (一台机器即可)  │             │ (一台机器即可)  │
  │                │             │                │
  │ 192.168.1.10   │             │ 10.0.0.10      │
  │   MySQL        │             │   MySQL        │
  │ 192.168.1.20   │             │ 10.0.0.20      │
  │   Redis        │             │   Redis        │
  │ 192.168.1.30   │             │ 10.0.0.30      │
  │   Jenkins      │             │   Grafana      │
  └────────────────┘             └────────────────┘
         ↑                               ↑
         └───── 中控面板统一管理 ─────────┘
                    ↓
              Nginx 反代 (443)
                    ↓
              用户浏览器
```

### 5.4 不需要的东西

| 原以为要做 | 为什么不需要 |
|---|---|
| `RemoveAllowedClient` 源码改造 | `Close()` 整个 Server = 白名单全清 |
| 强制断连机制 | `Close()` 自动断 |
| forward 轮询会话状态 | `Server.Close()` → `DialTCPPort` 报错 → 连接失败 |
| 修改 tailcat 源码 | 全程只用公开 API |
| token 加密存储 | token 相对不敏感，重点在客户端私钥的安全分发 |

---

## 6. Agent 设计

### 6.1 技术选型

Agent 必须是 Go（内嵌 tailcat Go 库）。控制面板与 agent 合一为 Go 单二进制：

- Go `net/http` + `html/template` 足以做审批流 + 会话管理面板
- 一个二进制部署，Docker 化简单
- 已有 Nginx 做反代，面板只需暴露 HTTP 端口
- 后期可换 SPA 前端，API 层不用改

### 6.2 API 设计

```
# 会话管理
POST   /api/sessions              # 创建会话（审批通过后调用）
GET    /api/sessions              # 列出活跃会话
GET    /api/sessions/{id}         # 查看会话详情
DELETE /api/sessions/{id}        # 关闭会话（TTL 到期或手动吊销）

# 资产管理
GET    /api/assets                # 列出资产
POST   /api/assets                # 录入资产
PUT    /api/assets/{id}           # 更新资产
DELETE /api/assets/{id}           # 删除资产

# 网关管理
GET    /api/gateways              # 列出网关
POST   /api/gateways              # 录入网关
GET    /api/gateways/{id}/status  # 网关健康状态

# 审批流
POST   /api/approvals             # 提交申请
PUT    /api/approvals/{id}        # 审批操作（approve/reject）
```

### 6.3 创建会话

请求：
```json
POST /api/sessions
{
  "asset_id": "mysql-prod-01",
  "port": 3306,
  "ttl_minutes": 60,
  "user": "zhangsan",
  "gateway_id": "mx-gw-01"
}
```

Agent 内部执行：
```go
// 1. 生成客户端密钥对（纯内存，不落盘）
priv := tailcat.NewPrivateKey()
pub := priv.Private.Public()

// 2. 查找资产实际 IP
asset := a.assets[assetID]

// 3. 创建独立 Server 实例（服务端转发模式）
s := &tailcat.Server{
    Key:            a.serverKey.Private,              // 网关固定密钥
    DERPMapURL:     a.derpMapURL,                      // 自建 DERP map
    ServedTCPPorts: []tailcfg.PortRange{{First: port, Last: port}},
    AllowedClients: []key.NodePublic{pub},             // 只允许这个客户端
    OnTCPForward: func(dst netip.AddrPort) (net.Conn, error) {
        // 服务端决定转发到实际资产 IP:port
        return net.Dial("tcp", fmt.Sprintf("%s:%d", asset.IP, asset.Port))
    },
}
s.Start()

// 4. 获取 token（自包含，内嵌 DERP 信息）
blob := s.ConnBlob()

// 5. TTL 到期自动回收
time.AfterFunc(ttl, func() { a.expireSession(sess.ID) })

// 6. 返回给用户
response := {
    "session_id":   "sess_xxx",
    "token":        blob,
    "client_key":   priv.String(),
    "expires_at":   time.Now().Add(60 * time.Minute),
    "connect_cmd":  fmt.Sprintf("tailcat-forward --client-key='%s' '%s' 13306:%d", priv.String(), blob, port),
}
```

### 6.4 会话回收

```go
func (a *Agent) expireSession(id string) {
    a.mu.Lock()
    defer a.mu.Unlock()
    sess, ok := a.sessions[id]
    if !ok { return }
    sess.Server.Close()              // 关闭 WireGuard → token 失效 → 连接断开
    delete(a.sessions, id)
    a.logf("session %s expired", id)
}
```

### 6.5 内部架构

```
┌─────────────────────────────────────────────┐
│  Agent（Go 单二进制）                          │
│                                               │
│  ┌──────────────┐   ┌──────────────────────┐ │
│  │  HTTP Server  │   │  Session Manager      │ │
│  │  (API + Web)  │──→│  map[string]*Session  │ │
│  │  :8443        │   │                      │ │
│  │  Routes:      │   │  Session{             │ │
│  │  /api/*       │   │    ID, User, TTL,    │ │
│  │  /  (Web UI)  │   │    Server, Token,    │ │
│  └──────────────┘   │    Asset, Port, ...  │ │
│                      │  }                   │ │
│  ┌──────────────┐   └──────────────────────┘ │
│  │ TTL Sweeper   │                             │
│  │ (AfterFunc)   │                             │
│  └──────────────┘                             │
│                                               │
│  ┌──────────────┐   ┌──────────────────────┐ │
│  │ Asset/Gateway│   │  DERP Map Cache       │ │
│  │ Store        │   │  (自建 DERP 配置)      │ │
│  └──────────────┘   └──────────────────────┘ │
└─────────────────────────────────────────────┘
```

### 6.6 代码骨架

```go
type Session struct {
    ID         string
    User       string
    Server     *tailcat.Server
    Token      tailcat.ConnBlob
    ClientPriv tailcat.PrivateKey
    AssetID    string
    AssetIP    string
    Port       int
    CreatedAt  time.Time
    ExpiresAt  time.Time
}

type Agent struct {
    mu        sync.Mutex
    sessions  map[string]*Session
    assets    map[string]*Asset
    gateways  map[string]*Gateway
    derpMapURL string
    serverKey  tailcat.PrivateKey  // 网关固定密钥
}

func (a *Agent) CreateSession(assetID string, port int, ttl time.Duration, user string) (*Session, error) {
    a.mu.Lock()
    defer a.mu.Unlock()

    priv := tailcat.NewPrivateKey()
    pub := priv.Private.Public()
    asset := a.assets[assetID]

    s := &tailcat.Server{
        Key:            a.serverKey.Private,
        DERPMapURL:     a.derpMapURL,
        ServedTCPPorts: []tailcfg.PortRange{{First: port, Last: port}},
        AllowedClients: []key.NodePublic{pub},
        OnTCPForward: func(dst netip.AddrPort) (net.Conn, error) {
            return net.Dial("tcp", fmt.Sprintf("%s:%d", asset.IP, asset.Port))
        },
    }
    if err := s.Start(); err != nil {
        return nil, fmt.Errorf("server start: %w", err)
    }

    sess := &Session{
        ID:         generateID(),
        User:       user,
        Server:     s,
        Token:      s.ConnBlob(),
        ClientPriv: priv,
        AssetID:    assetID,
        AssetIP:    asset.IP,
        Port:       port,
        CreatedAt:  time.Now(),
        ExpiresAt:  time.Now().Add(ttl),
    }

    time.AfterFunc(ttl, func() { a.expireSession(sess.ID) })

    a.sessions[sess.ID] = sess
    return sess, nil
}

func (a *Agent) expireSession(id string) {
    a.mu.Lock()
    defer a.mu.Unlock()
    sess, ok := a.sessions[id]
    if !ok { return }
    sess.Server.Close()
    delete(a.sessions, id)
}
```

---

## 7. tailcat-forward 改动

### 7.1 新增 `--client-key` 参数

当前 forward 只吃 token，不接受客户端私钥。改动点：

```go
// main.go 新增 flag
flagClientKey := flag.String("client-key", "", "客户端私钥（会话级身份）")

// 构造 client 时传入
func newClient(blob tailcat.ConnBlob, clientKeyStr string) *tailcat.Client {
    cl := tailcat.NewClient(blob)
    if clientKeyStr != "" {
        priv, err := tailcat.ParsePrivateKey(clientKeyStr)
        if err != nil { log.Fatalf("invalid client key: %v", err) }
        cl.Key = priv
    }
    return cl
}
```

改动量：~15 行。不传 `--client-key` 时行为不变（生成临时密钥），**向后兼容**。

### 7.2 用户命令

```bash
# PAM 系统下发的命令格式
tailcat-forward --client-key='nodekey:xxx' '<token>' 13306:3306

# 不带 client-key 的传统用法仍然兼容
tailcat-forward '<token>' 18080:8080
```

---

## 8. 部署方案

### 8.1 Agent Docker 部署

```yaml
# docker-compose.yml
services:
  tailcat-agent:
    build: .
    ports:
      - "8443:8443"
    volumes:
      - ./data:/app/data          # 资产/网关配置持久化
      - ./keys:/app/keys         # 网关固定密钥持久化
    environment:
      DERP_MAP_URL: "https://your-domain/derpmap.json"
      GATEWAY_KEY_FILE: "/app/keys/gateway.private.json"
    restart: always
```

### 8.2 Nginx 反代

追加到现有 nginx.conf：

```nginx
server {
    listen 443 ssl;
    server_name pam.jkwill.mx;
    ssl_certificate /etc/nginx/ssl/jkwill.mx.pem;
    ssl_certificate_key /etc/nginx/ssl/jkwill.mx.key;

    location / {
        proxy_pass http://127.0.0.1:8443;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 8.3 网关固定密钥初始化

首次部署 agent 时，生成网关固定密钥并持久化：

```bash
# 在网关机器上执行
tailcat genkey --key=default --region=<your-derp-region-id>
# 产出 /root/.config/tailcat/keys/default.private.json
# 将该文件挂载到 agent 容器的 /app/keys/
```

---

## 9. 飞书集成

### 方案 A：飞书审批流 API（推荐）
- 在飞书管理后台创建审批流程模板
- 中控通过飞书 OpenAPI 创建审批实例
- 审批通过后飞书回调中控 webhook → 触发 agent 创建会话
- 用户在飞书审批详情页直接看到 token + 连接命令

### 方案 B：站内审批 + 飞书消息通知
- 中控面板内置审批页面
- 审批人收到飞书消息卡片，点击跳转到面板审批
- 更灵活但需要用户登录面板

两种方案都需要飞书 OAuth 登录。

---

## 10. 实施路线图

### Phase 1：最小可用版（MVP）

| 任务 | 改动量 |
|---|---|
| forward 加 `--client-key` | ~15 行，改 main.go |
| Agent 原型（Go） | 新项目，~260 行 |
| 简单 Web 面板 | Go html/template |
| Docker 部署 | Dockerfile + compose.yaml |
| Nginx 反代 | 追加 server 配置 |

### Phase 2：生产化

| 任务 | 说明 |
|---|---|
| 飞书 OAuth 登录 | 接入飞书 OpenAPI |
| 飞书审批流 | 审批通过后自动触发会话创建 |
| 资产/网关管理 | 增删改查页面 |
| 审计日志 | 操作记录持久化 |
| 多网关支持 | agent 分布式部署 |

### Phase 3：可选增强

| 任务 | 说明 |
|---|---|
| `RemoveAllowedClient` 源码改造 | 支持不重启 Server 的单会话吊销 |
| 强制下线 | 管理员手动踢人 |
| 会话录像/流量统计 | 可选审计增强 |

---

## 11. 待产品侧决策的开放问题

- **网关 agent 部署位置**：每个区域 LAN 一台 agent 机器。
- **僵尸会话检测**：agent 异常退出后如何让中控感知并做状态修正。
- **审批超时策略**：审批人长时间不处理，是否需要自动提醒/升级/超时拒绝。

---

*本文档为最终版，源码调研 + 架构设计已完成。方案：不改 tailcat 源码，每会话独立 Server 实例 + Close() 回收 + OnTCPForward 服务端转发。Agent 用 Go 单二进制（API + Web 面板合一）。下一步进入 Phase 1 开发。*
