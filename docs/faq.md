# 常见问题（FAQ）

---

## 安装问题

### 安装脚本报错"不支持的操作系统"

脚本仅支持 CentOS 7+、Debian 9+、Ubuntu 18+。其他发行版请参考 [install.md](install.md) 中的手动安装步骤。

### 安装脚本报错"不支持的 CPU 架构"

仅支持 amd64 (x86_64) 和 arm64 (aarch64)。使用 `uname -m` 确认架构。

### 下载二进制文件失败

- 检查服务器是否能访问 GitHub：`curl -I https://github.com`
- 如果网络受限，可手动下载后上传到 `/usr/local/bin/anytls-server`

### 服务启动失败

1. 检查配置文件语法：确保 YAML 格式正确
2. 检查必填字段：`api_host`、`api_token`、`node_id` 不能为空
3. 检查端口占用：`ss -tlnp | grep 8443`
4. 查看详细日志：`journalctl -u anytls --no-pager -n 100`

---

## 连接问题

### 客户端无法连接服务端

1. 确认服务正在运行：`systemctl status anytls`
2. 确认防火墙已放行端口：`iptables -L -n | grep 8443`
3. 确认客户端配置的地址和端口正确
4. 使用 `telnet 服务器IP 8443` 测试端口连通性

### 认证失败 / 连接被转发到 fallback

- 确认客户端使用的 UUID 在面板用户列表中存在
- 确认用户未过期、未封禁、流量未超限
- 检查服务端日志中的认证失败记录
- 如果同一 IP 短时间内多次认证失败，可能被临时封禁（60 秒内失败超过 10 次会封禁 5 分钟）

---

## 证书相关问题

### 证书申请失败

- 确认域名已正确解析到服务器 IP：`ping your-domain.com`
- 确认 80 端口未被占用（acme.sh standalone 模式需要）：`ss -tlnp | grep :80`
- 确认服务器可以访问 Let's Encrypt：`curl -I https://acme-v02.api.letsencrypt.org`

### 证书过期

使用管理菜单续期：

```bash
bash install.sh
# 选择 10. 证书续期
```

或手动续期：

```bash
~/.acme.sh/acme.sh --renew-all --ecc --force
systemctl restart anytls
```

acme.sh 安装时已配置自动续期 cron job，正常情况下证书会自动续期。

### 使用自签名证书

不配置 `tls.cert_file` 和 `tls.key_file`（或留空），服务端会自动使用自签名证书。适用于测试或不需要域名的场景。

---

## 性能优化建议

### 系统参数调优

```bash
# 增加文件描述符限制（systemd 服务已配置 LimitNOFILE=65535）
# 如需进一步调整，编辑 /etc/security/limits.conf：
* soft nofile 65535
* hard nofile 65535
```

### 日志级别

生产环境建议使用 `info` 级别。`debug` 级别会产生大量日志，影响性能。

### Fallback 配置

配置 `fallback` 指向本机的 Web 服务（如 Nginx），可以：
- 防止主动探测识别代理服务
- 为认证失败的连接提供正常的 Web 响应

```yaml
fallback: "127.0.0.1:80"
```

---

## 用户常见疑问

## ERR_CONNECTION_CLOSED / 代理关闭且没有日志

常见原因：

- 密码错误，请检查您的密码是否正确。

## 好慢

网络速度与线路质量有关，升级更优质的线路可以提升速度。

请勿与 Hysteria 这类 udp/quic 协议做对比，因为它们底层的拥塞控制以及运营商侧应用的 QoS 策略不一样，完全没有可比性。

## 为什么选项这么少 / 为什么是自签证书

本项目只是提供一个简洁的 Any in TLS 代理的示例，并不旨在成为“通用代理工具”。

作为参考实现，不对 TLS 协议本身做过多的处理。

当然，如果你把这个协议集成到某些代理平台中，你将能够更好地控制 TLS ClientHello/ServerHello。

## FingerPrint 之类的选项呢

TLS 本身（ClientHello/ServerHello）的特征不是本项目关注的重点，现有的工具很容易改变这些特征。

- 某些 Golang 灵车代理早已标配 uTLS，为什么还被墙？
- 某些 Golang 灵车代理即使不用 uTLS，直接使用 Golang TLS 栈，为什么不被墙？

## 关于默认的 PaddingScheme

默认 PaddingScheme 只是一个示例。本项目无法确保默认参数不会被墙，因此设计了更新参数改变流量特征的机制。我们相信，实现成本低廉且易于改变的流量特征更有可能达到阻碍审查研究的目的。

## 如何更改 PaddingScheme

对接 Xboard 面板后，`padding_scheme` 通过面板节点配置下发，服务端会自动从 API 获取并应用，无需手动设置。

如果使用独立模式（未对接面板），可通过命令行参数指定：

```bash
anytls-server --padding-scheme ./padding.txt
```

## 还有别的 PaddingScheme 吗

模拟 XTLS-Vision:

```
stop=3
0=900-1400
1=900-1400
2=900-1400
```

模仿的不是特别像，但可以说明 XTLS-Vision 的弊端：写死的长度处理逻辑，只要 GFW 更新特征库就能识别。

## 命名疑问 / 更换传输层

事实上，如果您愿意，您可以将协议放置在其他传输加密层上，这需要一些代码，但不太多。

本协议主要负责的工作：

1. 合理的 TCP 连接复用与性能表现 (`proxy/session`)
2. 控制数据包长度模式，缓解“嵌套的TLS握手指纹识别” (`proxy/session` `proxy/padding`)

但是仅完成以上工作，仍然无法提供一个“好用”的代理。其他不得不完成的工作，例如加密，目前是依赖 TLS 完成的，因此协议取名为 AnyTLS。

更换其他传输层，您可能会失去 TLS 提供的安全保护。如果用于翻墙，还可能会触发不同的防火墙规则。

**除了“过 CDN”或“牺牲安全性来减少延迟”外，我想不出更换传输层的理由。如果你想尝试，请自行承担风险。**

## 参考过的项目

https://github.com/xtaci/smux （会话层与复用实现）

https://github.com/3andne/restls （PaddingScheme）

https://github.com/SagerNet/sing-box （代理框架）

https://github.com/klzgrad/naiveproxy （流量形态与流量分类调研）

## 已知弱点

以下弱点目前可能不会轻易引发“被墙”（甚至可能在大规模 DPI 下很难被利用），且修复可能引发协议不兼容，因此 anytls v1 没有对这些弱点进行处理。

- TLS over TLS 需要比普通 h2 请求更多的握手往返，也就是说，没有 h2 请求需要这么多的来回握手。除了进行 MITM 代理、破坏 E2E 加密之外，没有简单的方法可以避免这种情况。
- anytls 没有处理下行流量。虽然处理下行包特征会损失性能，但总的来说这一点很容易修复且不会破坏兼容性。
- anytls 现有的 PaddingScheme 语法对单个包的长度指定只有“单一固定长度”和“单一范围内随机”两种模式。此外 PaddingScheme 处理完毕后的剩余数据也只能直接发送。要修复这点，需要重新设计一套更复杂的 PaddingScheme 语法。
- anytls 几乎同时地发送三个或更多的数据包，特别是在 TLS 握手后的第一个 RTT 之内。即使单个数据包的长度符合某种被豁免的特征，也仍有可能被用于 `到达时间 - 包长 - 包数量` 和 `到达时间 - 通信数据量` 等统计。要修复这点，需要设置更多的缓冲区，实现将 auth 和 cmdSettings 等包合并发送，这会破坏现有 PaddingScheme 的语义。
- 即使修复了上一条所述的问题，包计数器仍然不一定能代表发包的时机，因为不可能预测被代理方的发送时机。
- 目前不清楚客户端初始化时使用默认 PaddingScheme 发起的少量连接，以及某些机械性的测试连接是否会对整体统计造成影响？
- 目前不清楚 GFW 对 TLS 连接会持续跟踪多久。
- TLS over TLS 开销导致可见的数据包长度增大和小数据包的缺失。消除这种开销还需要 MITM 代理。
- TLS over TLS 开销还会导致数据包持续超过 MTU 限制，这对于原始用户代理来说不应该发生。
- 由于这不是 HTTP 服务器，仍然可能存在主动探测问题，即使 gfw 对翻墙协议的主动探测似乎已不常见。
