# AnyTLS

一个试图缓解 嵌套的TLS握手指纹(TLS in TLS) 问题的代理协议。`anytls-go` 是该协议的参考实现。

- 灵活的分包和填充策略
- 连接复用，降低代理延迟
- 简洁的配置
- **支持对接 V2board 面板**（用户鉴权 / 流量上报）

[用户常见问题](./docs/faq.md)

[协议文档](./docs/protocol.md)

[URI 格式](./docs/uri_scheme.md)

## 快速食用方法

为了方便，示例服务器和客户端默认采用不安全的配置，该配置假设您不会遭遇 TLS 中间人攻击（这种情况偶尔发生在网络接入层，在骨干网络上几乎不可能实现）；否则，您的通信内容可能会被中间人截获。

### 一键安装（推荐，Linux）

提供自动化安装脚本，自动检测系统架构，下载最新版二进制，并配置为 systemd / OpenRC 系统服务。

```bash
bash <(wget -qO- https://raw.githubusercontent.com/code-gopher/anytls-go/main/install.sh) \
  --apiHost=https://your-panel.example.com \
  --apiKey=YOUR_API_KEY \
  --nodeID=1
```

安装完成后使用以下命令管理服务：

```bash
systemctl status anytls    # 查看状态
journalctl -u anytls -f   # 查看实时日志
systemctl restart anytls  # 重启
```

---

### 示例服务器（普通密码模式）

```
./anytls-server -l 0.0.0.0:8443 -p 密码
```

`0.0.0.0:8443` 为服务器监听的地址和端口。

### 示例服务器（V2board 面板模式）

接入 V2board 面板后，服务器会自动从面板获取监听端口、定期同步用户列表并上报流量。**客户端使用用户 UUID 作为密码**。

```
./anytls-server \
  --v2board-api-host https://your-panel.example.com \
  --v2board-api-key  YOUR_API_KEY \
  --v2board-node-id  1
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--v2board-api-host` | V2board 面板地址 | — |
| `--v2board-api-key` | API 密钥（token） | — |
| `--v2board-node-id` | 节点 ID | — |
| `--v2board-pull-interval` | 用户列表拉取周期 | `60s` |
| `--v2board-push-interval` | 流量上报周期 | `60s` |
| `-l` | 手动指定监听地址（覆盖面板配置） | 面板下发 |

> **注意**：普通密码模式（`-p`）与 V2board 模式互斥，二选一即可。

### 示例客户端

```
./anytls-client -l 127.0.0.1:1080 -s 服务器ip:端口 -p 密码
```

`127.0.0.1:1080` 为本机 Socks5 代理监听地址，理论上支持 TCP 和 UDP(通过 udp over tcp 传输)。

v0.0.12 版本起，示例客户端可直接使用 URI 格式:

```
./anytls-client -l 127.0.0.1:1080 -s "anytls://password@host:port"
```

### sing-box

https://github.com/SagerNet/sing-box

它包含了 anytls 协议的服务器和客户端。

### mihomo

https://github.com/MetaCubeX/mihomo

它包含了 anytls 协议的服务器和客户端。

### Shadowrocket

Shadowrocket 2.2.65+ 实现了 anytls 协议的客户端。
