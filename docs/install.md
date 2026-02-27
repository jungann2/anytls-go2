# AnytlsServer 安装部署文档

## 系统要求

| 项目 | 要求 |
|------|------|
| 操作系统 | CentOS 7+、Debian 9+、Ubuntu 18+ |
| CPU 架构 | amd64 (x86_64)、arm64 (aarch64) |
| 权限 | root 用户 |
| 网络 | 需要访问 GitHub（下载二进制）和 Xboard 面板 |
| 端口 | 默认 8443（可自定义），证书申请需要 80 端口 |

## 一键安装

### 在线安装

```bash
bash <(curl -sL https://raw.githubusercontent.com/anytls/anytls-go/main/install.sh) install
```

### 下载后安装

```bash
curl -O https://raw.githubusercontent.com/anytls/anytls-go/main/install.sh
chmod +x install.sh
bash install.sh install
```

安装脚本会自动完成以下步骤：
1. 检测操作系统和 CPU 架构
2. 安装依赖（curl、jq、socat）
3. 下载对应架构的二进制文件
4. 配置 systemd 服务
5. 启动交互式配置向导（输入域名、面板地址、Token、节点 ID）
6. 申请 TLS 证书（如提供域名）
7. 配置防火墙规则
8. 测试 API 连接并启动服务

## 手动安装

### 1. 下载二进制文件

```bash
# amd64
curl -L -o /usr/local/bin/anytls-server \
  https://github.com/anytls/anytls-go/releases/latest/download/anytls-server-linux-amd64

# arm64
curl -L -o /usr/local/bin/anytls-server \
  https://github.com/anytls/anytls-go/releases/latest/download/anytls-server-linux-arm64

chmod +x /usr/local/bin/anytls-server
```

### 2. 创建配置文件

```bash
mkdir -p /etc/anytls
cat > /etc/anytls/config.yaml << 'EOF'
listen: "0.0.0.0:8443"
api_host: "https://your-panel.com"
api_token: "your-server-token"
node_id: 1
node_type: "anytls"
tls:
  cert_file: "/etc/anytls/cert.pem"
  key_file: "/etc/anytls/key.pem"
log:
  level: "info"
  file_path: "/var/log/anytls/anytls.log"
fallback: "127.0.0.1:80"
EOF

chmod 600 /etc/anytls/config.yaml
mkdir -p /var/log/anytls
```

根据实际情况修改 `api_host`、`api_token`、`node_id` 等字段。详细配置说明见 [config.md](config.md)。

### 3. 创建 systemd 服务

```bash
cat > /etc/systemd/system/anytls.service << 'EOF'
[Unit]
Description=AnytlsServer - AnyTLS Proxy Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/anytls-server -c /etc/anytls/config.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable anytls
```

### 4. 启动服务

```bash
systemctl start anytls
systemctl status anytls
```

## 管理菜单

直接运行安装脚本（不带参数）即可进入管理菜单：

```bash
bash install.sh
```

菜单功能：

| 选项 | 功能 |
|------|------|
| 1 | 安装 AnytlsServer |
| 2 | 更新到最新版本 |
| 3 | 卸载 AnytlsServer |
| 4 | 启动服务 |
| 5 | 停止服务 |
| 6 | 重启服务 |
| 7 | 查看运行状态 |
| 8 | 查看实时日志 |
| 9 | 修改配置 |
| 10 | 证书续期 |

## 常用 systemctl 命令

```bash
systemctl start anytls     # 启动
systemctl stop anytls      # 停止
systemctl restart anytls   # 重启
systemctl status anytls    # 查看状态
journalctl -u anytls -f    # 查看实时日志
```
