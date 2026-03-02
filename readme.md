# AnyTLS Server

基于 [anytls-go](https://github.com/anytls/anytls-go) 改造的多用户代理服务端，支持 Xboard 面板对接和独立运行两种模式。

## 特性

- 支持 Xboard 面板多用户管理（用户同步、流量统计、在线追踪、设备限制）
- 支持独立运行模式（单用户，无需面板）
- 用户级限速（令牌桶算法）
- 连接速率限制（防暴力破解）
- 流量持久化（重启不丢失）
- TLS 证书自动申请（Let's Encrypt）
- 一键安装脚本
- 灵活的分包和填充策略（继承自 anytls 协议）
- 连接复用，降低代理延迟

## 快速安装

### 一键安装（推荐）

```bash
# 第一步：更新系统并安装必要依赖
sudo apt update -y && sudo apt install -y curl socat wget

# 第二步：运行一键安装脚本
bash <(curl -sL https://raw.githubusercontent.com/jungann2/anytls-go2/main/install.sh) install
```

安装过程中会让你选择：
- **独立模式**：自动生成密码和端口，启动后直接输出分享链接，粘贴到客户端即可使用
- **Xboard 模式**：输入面板地址、Token、节点 ID，对接 Xboard 面板管理

### 手动安装

```bash
# 克隆仓库
git clone https://github.com/jungann2/anytls-go2.git
cd anytls-go2

# 编译（需要 Go 1.21+）
go build -o anytls-server ./cmd/server/

# 移动到系统目录
sudo mv anytls-server /usr/local/bin/
```

## 使用方式

### 独立模式（快速测试）

```bash
# 命令行直接运行
anytls-server -standalone -p "你的密码" -l "0.0.0.0:8443"

# 或使用配置文件
anytls-server -c /etc/anytls/config.yaml
```

独立模式配置文件示例：
```yaml
standalone: true
password: "your-password-here"
listen: "0.0.0.0:8443"
log:
  level: "info"
```

启动后会自动打印 `anytls://` 分享链接和 Clash 配置片段。

### Xboard 面板模式

```bash
anytls-server -c /etc/anytls/config.yaml
```

Xboard 模式配置文件示例：
```yaml
listen: "0.0.0.0:8443"
api_host: "https://your-panel.com"
api_token: "your-communication-token"
node_id: 1
node_type: "anytls"
tls:
  cert_file: "/etc/anytls/cert.pem"
  key_file: "/etc/anytls/key.pem"
log:
  level: "info"
  file_path: "/var/log/anytls/anytls.log"
fallback: "127.0.0.1:80"
```

## 管理脚本

安装后直接运行脚本即可进入管理菜单：

```bash
bash <(curl -sL https://raw.githubusercontent.com/jungann2/anytls-go2/main/install.sh)
```

功能包括：启动/停止/重启、查看状态和日志、修改密码和端口、查看分享链接、证书续期、更新版本、卸载。

## 客户端支持

- [FlClash](https://github.com/chen08209/FlClash) / [Clash.Meta (mihomo)](https://github.com/MetaCubeX/mihomo) - 支持 anytls 协议
- [sing-box](https://github.com/SagerNet/sing-box) - 包含 anytls 客户端和服务端
- [Shadowrocket](https://apps.apple.com/app/shadowrocket/id932747118) 2.2.65+ - 支持 anytls 协议
- anytls 官方示例客户端

### URI 分享格式

```
anytls://password@server:port/?insecure=1&sni=example.com
```

## 文档

- [安装部署文档](./docs/install.md)
- [Xboard 对接文档](./docs/xboard.md)
- [配置文件说明](./docs/config.md)
- [常见问题](./docs/faq.md)
- [协议文档](./docs/protocol.md)
- [URI 格式](./docs/uri_scheme.md)

## 致谢

本项目基于 [anytls-go](https://github.com/anytls/anytls-go) 开发，感谢原作者的协议设计和参考实现。
