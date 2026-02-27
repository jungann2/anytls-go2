# 配置文件说明

配置文件路径：`/etc/anytls/config.yaml`，权限建议设为 `600`。

## 字段说明

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `listen` | string | 否 | `"0.0.0.0:8443"` | 监听地址和端口 |
| `api_host` | string | **是** | — | Xboard 面板地址，如 `https://your-panel.com` |
| `api_token` | string | **是** | — | 服务端通讯密钥 |
| `node_id` | int | **是** | — | 节点 ID，必须大于 0 |
| `node_type` | string | 否 | `"anytls"` | 节点类型，固定为 `anytls` |
| `tls.cert_file` | string | 否 | `""` | TLS 证书文件路径 |
| `tls.key_file` | string | 否 | `""` | TLS 私钥文件路径 |
| `log.level` | string | 否 | `"info"` | 日志级别：`debug`、`info`、`warn`、`error` |
| `log.file_path` | string | 否 | `""` | 日志文件路径，为空则仅输出到标准输出 |
| `fallback` | string | 否 | `""` | 认证失败时的转发目标地址 |

## 完整配置示例

```yaml
# AnytlsServer 配置文件

# 监听地址
listen: "0.0.0.0:8443"

# Xboard 面板对接
api_host: "https://your-panel.com"
api_token: "your-server-token"
node_id: 1
node_type: "anytls"

# TLS 配置
tls:
  cert_file: "/etc/anytls/cert.pem"
  key_file: "/etc/anytls/key.pem"

# 日志配置
log:
  level: "info"
  file_path: "/var/log/anytls/anytls.log"

# Fallback 配置（认证失败时转发到此地址，用于防主动探测）
fallback: "127.0.0.1:80"
```

## 最小配置示例

仅包含必填字段，其余使用默认值：

```yaml
api_host: "https://your-panel.com"
api_token: "your-server-token"
node_id: 1
```

此配置下：
- 监听 `0.0.0.0:8443`
- 使用自签名 TLS 证书
- 日志级别为 `info`，仅输出到标准输出
- 无 fallback 转发

## TLS 证书配置

### 使用外部证书

指定证书和私钥文件路径：

```yaml
tls:
  cert_file: "/etc/anytls/cert.pem"
  key_file: "/etc/anytls/key.pem"
```

支持 Let's Encrypt 等 CA 签发的证书。使用一键安装脚本时可自动通过 acme.sh 申请证书。

### 使用自签名证书

不配置 `tls` 或留空路径，服务端会自动生成自签名证书：

```yaml
tls:
  cert_file: ""
  key_file: ""
```

> 自签名证书适用于测试环境。生产环境建议使用正式证书。

### 证书加载失败

如果指定的证书文件不存在或格式无效，服务端会回退到自签名证书并在日志中记录警告。

## 日志配置

### 日志级别

| 级别 | 说明 |
|------|------|
| `debug` | 详细调试信息，包含连接细节 |
| `info` | 常规运行信息（推荐） |
| `warn` | 警告信息，如证书加载失败回退 |
| `error` | 错误信息，如 API 请求失败 |

### 日志输出

- 配置 `file_path` 后，日志同时输出到文件和标准输出
- 不配置 `file_path` 时，仅输出到标准输出
- 日志使用 JSON 格式，便于日志收集和分析

```yaml
log:
  level: "info"
  file_path: "/var/log/anytls/anytls.log"
```

确保日志目录存在：

```bash
mkdir -p /var/log/anytls
```

## 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-c` | 指定配置文件路径 | `/etc/anytls/config.yaml` |

```bash
anytls-server -c /path/to/config.yaml
```
