# Xboard 对接配置文档

## 前置条件

- 已部署 Xboard 面板并正常运行
- 已安装 AnytlsServer（参考 [install.md](install.md)）
- Xboard 面板已支持 AnyTLS 协议

## 面板端配置

### 1. 获取服务端通讯密钥

1. 登录 Xboard 管理后台
2. 进入「系统配置」→「服务端通讯密钥」
3. 复制通讯密钥（即 `api_token`）

如果尚未设置通讯密钥，请先生成一个并保存。

### 2. 添加 AnyTLS 节点

1. 进入「节点管理」
2. 点击「添加节点」
3. 协议选择「AnyTLS」
4. 填写节点信息：
   - **节点名称**：自定义名称
   - **节点地址**：服务器域名或 IP
   - **连接端口**：与服务端 `listen` 端口一致（默认 8443）
5. 保存节点，记录节点 ID

### 3. 配置 padding_scheme（可选）

在 Xboard 节点设置中可以配置 `padding_scheme`，用于控制流量特征。格式为 JSON 数组：

```json
["stop=8", "0=30-30", "1=100-400"]
```

AnytlsServer 会在启动时和每次 pull 周期从面板自动获取此配置，无需在服务端手动设置。

## 服务端配置

### 关键配置项

在 `/etc/anytls/config.yaml` 中配置以下字段：

```yaml
api_host: "https://your-panel.com"   # Xboard 面板地址（不带末尾斜杠）
api_token: "your-server-token"       # 服务端通讯密钥（与面板一致）
node_id: 1                           # 面板中的节点 ID
node_type: "anytls"                  # 固定为 anytls
```

### node_id 说明

`node_id` 必须与 Xboard 面板中对应节点的 ID 完全一致。可以在面板「节点管理」列表中查看每个节点的 ID。

配置错误的 `node_id` 会导致：
- API 请求返回 404 或错误数据
- 用户列表拉取失败
- 流量统计无法正确归属

### 通讯密钥说明

`api_token` 是服务端与面板通信的认证凭证，必须与面板「系统配置」中的「服务端通讯密钥」完全一致。

## 对接验证

### 1. 测试 API 连接

```bash
curl -s "https://your-panel.com/api/v1/server/UniProxy/config?token=YOUR_TOKEN&node_id=1&node_type=anytls"
```

正常返回示例：

```json
{
  "server_port": 8443,
  "server_name": "example.com",
  "padding_scheme": ["stop=8", "0=30-30"],
  "base_config": {
    "push_interval": 60,
    "pull_interval": 60
  }
}
```

### 2. 检查服务日志

```bash
journalctl -u anytls --no-pager -n 50
```

正常启动后应看到类似日志：
- 成功获取节点配置
- 成功拉取用户列表
- 开始监听端口

## 常见对接问题

### API 连接失败

**现象**：日志中出现 API 请求超时或连接拒绝

排查步骤：
1. 确认面板地址可从服务器访问：`curl -I https://your-panel.com`
2. 确认 `api_token` 正确
3. 确认面板 HTTPS 证书有效
4. 检查服务器出站防火墙是否放行 443 端口

### 用户列表为空

**现象**：服务启动正常但无法认证任何用户

排查步骤：
1. 确认 `node_id` 与面板中的节点 ID 一致
2. 确认面板中该节点下有分配用户
3. 确认用户未过期、未封禁、流量未超限
4. 手动调用用户接口检查返回数据

### 流量统计不显示

**现象**：用户可以正常使用但面板不显示流量

排查步骤：
1. 检查日志中是否有流量上报错误
2. 确认 `push_interval` 配置正常（默认 60 秒）
3. 等待一个上报周期后刷新面板
4. 确认面板中节点的倍率（rate）设置正确

### 节点显示离线

**现象**：面板中节点状态显示为离线

排查步骤：
1. 确认 AnytlsServer 正在运行：`systemctl status anytls`
2. 确认状态上报接口正常工作
3. 检查服务器时间是否准确
