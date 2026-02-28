#!/bin/bash
# ============================================================
# AnytlsServer 一键安装管理脚本
# 描述：AnytlsServer - AnyTLS 代理服务端一键部署与管理
# 作者：anytls
# Github：https://github.com/anytls/anytls-go
# ============================================================

# 严格模式
set -euo pipefail

# ============================================================
# 全局变量定义
# ============================================================

# 版本信息
SCRIPT_VERSION="v1.0.0"

# 安装路径
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="anytls-server"
BINARY_PATH="${INSTALL_DIR}/${BINARY_NAME}"

# 配置路径
CONFIG_DIR="/etc/anytls"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
CERT_FILE="${CONFIG_DIR}/cert.pem"
KEY_FILE="${CONFIG_DIR}/key.pem"

# 日志路径
LOG_DIR="/var/log/anytls"
LOG_FILE="${LOG_DIR}/anytls.log"

# Systemd 服务
SERVICE_NAME="anytls"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# GitHub Release 下载地址
GITHUB_REPO="jungann2/anytls-go2"
DOWNLOAD_URL_BASE="https://github.com/${GITHUB_REPO}/releases/latest/download"

# 系统信息
OS_RELEASE=""
ARCH=""
INSTALL_CMD=""
REMOVE_CMD=""

# 颜色输出类型
ECHO_TYPE="echo -e"

# ============================================================
# 颜色输出函数
# ============================================================

echoContent() {
    case $1 in
    "red")
        ${ECHO_TYPE} "\033[31m$2\033[0m"
        ;;
    "green")
        ${ECHO_TYPE} "\033[32m$2\033[0m"
        ;;
    "yellow")
        ${ECHO_TYPE} "\033[33m$2\033[0m"
        ;;
    "skyBlue")
        ${ECHO_TYPE} "\033[1;36m$2\033[0m"
        ;;
    "white")
        ${ECHO_TYPE} "\033[37m$2\033[0m"
        ;;
    esac
}

# ============================================================
# 基础检查函数
# ============================================================

# 检查 root 权限
check_root() {
    if [[ "$(id -u)" != "0" ]]; then
        echoContent red "错误：请使用 root 用户运行此脚本"
        exit 1
    fi
}

# 检测操作系统
check_system() {
    if [[ -n $(find /etc -name "redhat-release" 2>/dev/null) ]] || grep -qi "centos" /proc/version 2>/dev/null; then
        OS_RELEASE="centos"
        INSTALL_CMD="yum -y install"
        REMOVE_CMD="yum -y remove"

        # 检查 CentOS 版本
        if [[ -f "/etc/centos-release" ]]; then
            local version
            version=$(rpm -q centos-release 2>/dev/null | awk -F "[-]" '{print $3}' | awk -F "[.]" '{print $1}')
            if [[ -n "${version}" && "${version}" -lt 7 ]]; then
                echoContent red "错误：不支持 CentOS ${version}，需要 CentOS 7+"
                exit 1
            fi
        fi

    elif grep -qi "debian" /etc/issue 2>/dev/null || grep -qi "debian" /proc/version 2>/dev/null; then
        OS_RELEASE="debian"
        INSTALL_CMD="apt -y install"
        REMOVE_CMD="apt -y autoremove"

    elif grep -qi "ubuntu" /etc/issue 2>/dev/null || grep -qi "ubuntu" /proc/version 2>/dev/null; then
        OS_RELEASE="ubuntu"
        INSTALL_CMD="apt -y install"
        REMOVE_CMD="apt -y autoremove"

    else
        echoContent red "错误：不支持的操作系统"
        echoContent yellow "支持的系统：CentOS 7+、Debian 9+、Ubuntu 18+"
        if [[ -f /etc/issue ]]; then
            echoContent yellow "当前系统：$(cat /etc/issue)"
        fi
        exit 1
    fi

    echoContent green "操作系统检测：${OS_RELEASE}"
}

# 检测 CPU 架构
check_arch() {
    case "$(uname -m)" in
    'x86_64' | 'amd64')
        ARCH="amd64"
        ;;
    'aarch64' | 'armv8' | 'arm64')
        ARCH="arm64"
        ;;
    *)
        echoContent red "错误：不支持的 CPU 架构 $(uname -m)"
        echoContent yellow "支持的架构：amd64 (x86_64)、arm64 (aarch64)"
        exit 1
        ;;
    esac

    echoContent green "CPU 架构检测：${ARCH}"
}

# 安装系统依赖
install_dependencies() {
    echoContent skyBlue "正在安装系统依赖..."

    # 更新包管理器
    if [[ "${OS_RELEASE}" == "centos" ]]; then
        yum update -y >/dev/null 2>&1
    else
        apt update -y >/dev/null 2>&1
    fi

    # 安装 curl
    if ! command -v curl >/dev/null 2>&1; then
        echoContent yellow "  安装 curl..."
        ${INSTALL_CMD} curl >/dev/null 2>&1
    fi

    # 安装 jq
    if ! command -v jq >/dev/null 2>&1; then
        echoContent yellow "  安装 jq..."
        ${INSTALL_CMD} jq >/dev/null 2>&1
    fi

    # 安装 socat（acme.sh 需要）
    if ! command -v socat >/dev/null 2>&1; then
        echoContent yellow "  安装 socat..."
        ${INSTALL_CMD} socat >/dev/null 2>&1
    fi

    echoContent green "系统依赖安装完成"
}

# ============================================================
# 核心安装功能
# ============================================================

# 下载二进制文件
download_binary() {
    echoContent skyBlue "正在下载 AnytlsServer..."

    local download_url="${DOWNLOAD_URL_BASE}/anytls-server-linux-${ARCH}"

    # 创建临时文件
    local tmp_file="/tmp/anytls-server-download"

    if ! curl -L -o "${tmp_file}" "${download_url}" --progress-bar; then
        echoContent red "错误：下载失败，请检查网络连接"
        echoContent yellow "下载地址：${download_url}"
        rm -f "${tmp_file}"
        exit 1
    fi

    # 移动到安装目录
    mv -f "${tmp_file}" "${BINARY_PATH}"
    chmod +x "${BINARY_PATH}"

    echoContent green "AnytlsServer 下载完成：${BINARY_PATH}"
}

# 创建 systemd 服务
setup_systemd() {
    echoContent skyBlue "正在配置 systemd 服务..."

    cat >"${SERVICE_FILE}" <<EOF
[Unit]
Description=AnytlsServer - AnyTLS Proxy Server
After=network.target

[Service]
Type=simple
ExecStart=${BINARY_PATH} -c ${CONFIG_FILE}
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    # 重新加载 systemd 并设置开机自启
    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}" >/dev/null 2>&1

    echoContent green "systemd 服务配置完成"
}

# 配置防火墙
setup_firewall() {
    local port="$1"

    echoContent skyBlue "正在配置防火墙规则（端口 ${port}）..."

    # firewalld
    if command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active firewalld >/dev/null 2>&1; then
        firewall-cmd --permanent --add-port="${port}/tcp" >/dev/null 2>&1
        firewall-cmd --reload >/dev/null 2>&1
        echoContent green "  firewalld 规则已添加"
        return
    fi

    # ufw
    if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "active"; then
        ufw allow "${port}/tcp" >/dev/null 2>&1
        echoContent green "  ufw 规则已添加"
        return
    fi

    # iptables（兜底方案）
    if command -v iptables >/dev/null 2>&1; then
        if ! iptables -C INPUT -p tcp --dport "${port}" -j ACCEPT >/dev/null 2>&1; then
            iptables -I INPUT -p tcp --dport "${port}" -j ACCEPT >/dev/null 2>&1
            echoContent green "  iptables 规则已添加"
        else
            echoContent yellow "  iptables 规则已存在"
        fi
        return
    fi

    echoContent yellow "  未检测到防火墙，跳过配置"
}

# ============================================================
# 证书管理
# ============================================================

# 安装 acme.sh
install_acme() {
    if [[ -f "$HOME/.acme.sh/acme.sh" ]]; then
        echoContent green "acme.sh 已安装，跳过"
        return
    fi

    echoContent skyBlue "正在安装 acme.sh..."

    if ! curl -s https://get.acme.sh | sh -s email="anytls@example.com" >/dev/null 2>&1; then
        echoContent red "错误：acme.sh 安装失败"
        exit 1
    fi

    echoContent green "acme.sh 安装完成"
}

# 申请 TLS 证书
apply_cert() {
    local domain="$1"

    if [[ -z "${domain}" ]]; then
        echoContent red "错误：域名不能为空"
        return 1
    fi

    echoContent skyBlue "正在为 ${domain} 申请 TLS 证书..."

    # 安装 acme.sh
    install_acme

    # 确保配置目录存在
    mkdir -p "${CONFIG_DIR}"

    # 使用 standalone 模式申请证书
    "$HOME/.acme.sh/acme.sh" --set-default-ca --server letsencrypt >/dev/null 2>&1

    if ! "$HOME/.acme.sh/acme.sh" --issue -d "${domain}" --standalone --keylength ec-256 --force; then
        echoContent red "错误：证书申请失败"
        echoContent yellow "请确保："
        echoContent yellow "  1. 域名已正确解析到本机 IP"
        echoContent yellow "  2. 80 端口未被占用"
        return 1
    fi

    # 安装证书到指定路径
    if ! "$HOME/.acme.sh/acme.sh" --install-cert -d "${domain}" --ecc \
        --fullchain-file "${CERT_FILE}" \
        --key-file "${KEY_FILE}" \
        --reloadcmd "systemctl restart ${SERVICE_NAME} 2>/dev/null || true"; then
        echoContent red "错误：证书安装失败"
        return 1
    fi

    # 设置证书文件权限
    chmod 644 "${CERT_FILE}"
    chmod 600 "${KEY_FILE}"

    echoContent green "TLS 证书申请成功"
    echoContent green "  证书文件：${CERT_FILE}"
    echoContent green "  私钥文件：${KEY_FILE}"
    echoContent green "  自动续期已通过 acme.sh cron job 配置"
}

# 手动续期证书
renew_cert() {
    if [[ ! -f "$HOME/.acme.sh/acme.sh" ]]; then
        echoContent red "错误：acme.sh 未安装，无法续期"
        return 1
    fi

    echoContent skyBlue "正在续期 TLS 证书..."

    if "$HOME/.acme.sh/acme.sh" --renew-all --ecc --force; then
        echoContent green "证书续期成功"
    else
        echoContent red "证书续期失败，请检查域名解析和网络"
    fi
}

# ============================================================
# 工具函数
# ============================================================

# 生成随机密码（UUID 格式）
generate_password() {
    if command -v uuidgen >/dev/null 2>&1; then
        uuidgen
    else
        cat /proc/sys/kernel/random/uuid 2>/dev/null || head -c 32 /dev/urandom | md5sum | head -c 32
    fi
}

# 生成随机端口（10000-60000）
generate_port() {
    shuf -i 10000-60000 -n 1 2>/dev/null || echo $(( RANDOM % 50000 + 10000 ))
}

# 获取公网 IP
get_public_ip() {
    curl -s --connect-timeout 5 https://api.ipify.org 2>/dev/null || \
    curl -s --connect-timeout 5 https://ifconfig.me 2>/dev/null || \
    curl -s --connect-timeout 5 https://ip.sb 2>/dev/null || \
    echo ""
}

# 从配置文件读取值
get_config_value() {
    local key="$1"
    if [[ -f "${CONFIG_FILE}" ]]; then
        grep "^${key}:" "${CONFIG_FILE}" 2>/dev/null | head -1 | sed 's/^[^:]*: *//;s/^"//;s/"$//'
    fi
}

# ============================================================
# 交互式配置向导
# ============================================================

# 安装模式选择
choose_install_mode() {
    echoContent skyBlue "\n请选择安装模式："
    echoContent red "=============================================================="
    echoContent yellow "1. 独立模式 - 单用户，不依赖面板，快速测试"
    echoContent yellow "2. Xboard 模式 - 多用户，对接 Xboard 面板管理"
    echoContent red "=============================================================="

    local mode=""
    while [[ "${mode}" != "1" && "${mode}" != "2" ]]; do
        read -r -p "请选择 [1/2]: " mode
    done

    if [[ "${mode}" == "1" ]]; then
        setup_standalone_config
    else
        setup_xboard_config
    fi
}

# 独立模式配置
setup_standalone_config() {
    echoContent skyBlue "\n配置独立模式..."
    echoContent red "=============================================================="

    # 密码
    local default_password
    default_password=$(generate_password)
    read -r -p "请输入密码（留空自动生成）: " input_password
    local password="${input_password:-${default_password}}"

    # 端口
    local default_port
    default_port=$(generate_port)
    read -r -p "请输入监听端口（留空随机生成，默认 ${default_port}）: " input_port
    local listen_port="${input_port:-${default_port}}"

    # 域名（可选）
    local domain=""
    read -r -p "请输入域名（用于 TLS 证书，留空使用自签名证书）: " domain

    # 创建目录
    mkdir -p "${CONFIG_DIR}"
    mkdir -p "${LOG_DIR}"

    # TLS 配置
    local cert_config="tls:
  cert_file: \"\"
  key_file: \"\""
    if [[ -n "${domain}" ]]; then
        cert_config="tls:
  cert_file: \"${CERT_FILE}\"
  key_file: \"${KEY_FILE}\""
    fi

    # 生成配置文件
    cat >"${CONFIG_FILE}" <<EOF
# AnytlsServer 配置文件 - 独立模式
# 由安装脚本自动生成

standalone: true
password: "${password}"
listen: "0.0.0.0:${listen_port}"
node_type: "anytls"

# TLS 配置
${cert_config}

# 日志配置
log:
  level: "info"
  file_path: "${LOG_FILE}"

# Fallback 配置（认证失败时转发到此地址）
fallback: "127.0.0.1:80"
EOF

    chmod 600 "${CONFIG_FILE}"
    echoContent green "\n配置文件已生成：${CONFIG_FILE}"

    # 申请证书
    if [[ -n "${domain}" ]]; then
        apply_cert "${domain}"
    else
        echoContent yellow "跳过证书申请，将使用自签名证书"
    fi

    # 配置防火墙
    setup_firewall "${listen_port}"

    # 显示分享链接
    show_share_link "${password}" "${listen_port}" "${domain}"
}

# Xboard 模式配置
setup_xboard_config() {
    echoContent skyBlue "\n配置 Xboard 模式..."
    echoContent red "=============================================================="

    # 输入域名
    local domain=""
    read -r -p "请输入域名（用于 TLS 证书，留空跳过证书申请）: " domain

    # 输入 API 地址
    local api_host=""
    while [[ -z "${api_host}" ]]; do
        read -r -p "请输入 Xboard 面板地址（如 https://your-panel.com）: " api_host
        if [[ -z "${api_host}" ]]; then
            echoContent red "面板地址不能为空"
        fi
    done
    api_host="${api_host%/}"

    # 输入 token
    local api_token=""
    while [[ -z "${api_token}" ]]; do
        read -r -p "请输入通信 Token（Xboard 后台 > 系统配置 > 服务端通讯密钥）: " api_token
        if [[ -z "${api_token}" ]]; then
            echoContent red "Token 不能为空"
        fi
    done

    # 输入 node_id
    local node_id=""
    while [[ -z "${node_id}" || ! "${node_id}" =~ ^[0-9]+$ ]]; do
        read -r -p "请输入节点 ID（Xboard 后台 > 节点管理中的节点 ID）: " node_id
        if [[ ! "${node_id}" =~ ^[0-9]+$ ]]; then
            echoContent red "节点 ID 必须为正整数"
        fi
    done

    # 输入监听端口
    local listen_port="8443"
    read -r -p "请输入监听端口（默认 8443）: " input_port
    if [[ -n "${input_port}" && "${input_port}" =~ ^[0-9]+$ ]]; then
        listen_port="${input_port}"
    fi

    # 创建目录
    mkdir -p "${CONFIG_DIR}"
    mkdir -p "${LOG_DIR}"

    # TLS 配置
    local cert_config="tls:
  cert_file: \"\"
  key_file: \"\""
    if [[ -n "${domain}" ]]; then
        cert_config="tls:
  cert_file: \"${CERT_FILE}\"
  key_file: \"${KEY_FILE}\""
    fi

    # 生成配置文件
    cat >"${CONFIG_FILE}" <<EOF
# AnytlsServer 配置文件 - Xboard 模式
# 由安装脚本自动生成

listen: "0.0.0.0:${listen_port}"

# Xboard 面板对接
api_host: "${api_host}"
api_token: "${api_token}"
node_id: ${node_id}
node_type: "anytls"

# TLS 配置
${cert_config}

# 日志配置
log:
  level: "info"
  file_path: "${LOG_FILE}"

# Fallback 配置（认证失败时转发到此地址）
fallback: "127.0.0.1:80"
EOF

    chmod 600 "${CONFIG_FILE}"
    echoContent green "\n配置文件已生成：${CONFIG_FILE}"

    # 申请证书
    if [[ -n "${domain}" ]]; then
        apply_cert "${domain}"
    else
        echoContent yellow "跳过证书申请，将使用自签名证书"
    fi

    # 配置防火墙
    setup_firewall "${listen_port}"

    # 测试 API 连接
    test_connection "${api_host}" "${api_token}" "${node_id}"

    # 显示 Xboard 节点配置说明
    show_xboard_guide "${listen_port}" "${domain}"
}

# 配置向导（兼容旧入口）
setup_config() {
    choose_install_mode
}

# 显示分享链接
show_share_link() {
    local password="$1"
    local port="$2"
    local domain="${3:-}"

    local server_ip
    server_ip=$(get_public_ip)
    local host="${domain:-${server_ip:-YOUR_SERVER_IP}}"
    local sni_param=""
    local sni_line=""
    if [[ -n "${domain}" ]]; then
        sni_param="&sni=${domain}"
        sni_line="    sni: \"${domain}\""
    fi

    echoContent red "\n=============================================================="
    echoContent green "========== AnyTLS 分享链接 =========="
    echoContent skyBlue "anytls://${password}@${host}:${port}/?insecure=1${sni_param}"
    echoContent green "======================================"
    echoContent green ""
    echoContent green "FlClash / Clash.Meta 配置片段："
    echoContent yellow "  - name: \"anytls-node\""
    echoContent yellow "    type: anytls"
    echoContent yellow "    server: ${host}"
    echoContent yellow "    port: ${port}"
    echoContent yellow "    password: \"${password}\""
    echoContent yellow "    udp: true"
    echoContent yellow "    skip-cert-verify: true"
    if [[ -n "${sni_line}" ]]; then
        echoContent yellow "${sni_line}"
    fi
    echoContent red "=============================================================="
}

# 测试 API 连接
test_connection() {
    local api_host="$1"
    local api_token="$2"
    local node_id="$3"

    echoContent skyBlue "\n正在测试与 Xboard API 的连接..."

    local test_url="${api_host}/api/v1/server/UniProxy/config?token=${api_token}&node_id=${node_id}&node_type=anytls"

    local http_code
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 "${test_url}" 2>/dev/null)

    if [[ "${http_code}" == "200" ]]; then
        echoContent green "API 连接测试成功 (HTTP ${http_code})"
    elif [[ "${http_code}" == "000" ]]; then
        echoContent red "API 连接测试失败：无法连接到服务器"
        echoContent yellow "请检查面板地址是否正确且可访问"
    else
        echoContent red "API 连接测试失败 (HTTP ${http_code})"
        echoContent yellow "请检查 Token 和节点 ID 是否正确"
    fi
}

# 显示 Xboard 节点配置说明
show_xboard_guide() {
    local port="$1"
    local domain="$2"

    echoContent skyBlue "\n=============================================================="
    echoContent green "在 Xboard 面板中添加 AnyTLS 节点的步骤："
    echoContent yellow "  1. 登录 Xboard 管理后台"
    echoContent yellow "  2. 进入「节点管理」"
    echoContent yellow "  3. 添加新节点，协议选择「AnyTLS」"
    echoContent yellow "  4. 填写节点信息："
    if [[ -n "${domain}" ]]; then
        echoContent yellow "     - 节点地址：${domain}"
    else
        echoContent yellow "     - 节点地址：<服务器 IP>"
    fi
    echoContent yellow "     - 连接端口：${port}"
    echoContent yellow "  5. 保存后确保节点 ID 与配置文件中的 node_id 一致"
    echoContent skyBlue "=============================================================="
}

# 修改配置
modify_config() {
    if [[ ! -f "${CONFIG_FILE}" ]]; then
        echoContent red "错误：配置文件不存在，请先安装"
        return
    fi

    echoContent skyBlue "当前配置文件：${CONFIG_FILE}"
    echoContent yellow "请选择要修改的项目："
    echoContent yellow "1. 使用编辑器打开配置文件"
    echoContent yellow "2. 重新运行配置向导"
    echoContent yellow "3. 修改密码（独立模式）"
    echoContent yellow "4. 修改端口"
    echoContent red "=============================================================="

    read -r -p "请选择: " modify_choice

    case ${modify_choice} in
    1)
        local editor="vi"
        if command -v nano >/dev/null 2>&1; then
            editor="nano"
        fi
        ${editor} "${CONFIG_FILE}"
        echoContent green "配置已修改，请重启服务使其生效"
        ;;
    2)
        choose_install_mode
        echoContent yellow "请重启服务使新配置生效: systemctl restart ${SERVICE_NAME}"
        ;;
    3)
        modify_password
        ;;
    4)
        modify_port
        ;;
    *)
        echoContent red "无效选择"
        ;;
    esac
}

# 修改密码（独立模式）
modify_password() {
    local is_standalone
    is_standalone=$(get_config_value "standalone")
    if [[ "${is_standalone}" != "true" ]]; then
        echoContent red "Xboard 模式下密码由面板管理，无需手动修改"
        return
    fi

    local current_password
    current_password=$(get_config_value "password")
    echoContent yellow "当前密码：${current_password}"

    read -r -p "请输入新密码（留空随机生成）: " new_password
    if [[ -z "${new_password}" ]]; then
        new_password=$(generate_password)
    fi

    sed -i "s|^password:.*|password: \"${new_password}\"|" "${CONFIG_FILE}"
    echoContent green "密码已修改为：${new_password}"
    echoContent yellow "正在重启服务..."
    systemctl restart "${SERVICE_NAME}" 2>/dev/null || true

    # 显示新的分享链接
    local listen_port
    listen_port=$(get_config_value "listen" | grep -oP ':\K[0-9]+')
    show_share_link "${new_password}" "${listen_port}" ""
}

# 修改端口
modify_port() {
    local current_listen
    current_listen=$(get_config_value "listen")
    echoContent yellow "当前监听地址：${current_listen}"

    read -r -p "请输入新端口: " new_port
    if [[ ! "${new_port}" =~ ^[0-9]+$ ]]; then
        echoContent red "端口必须为数字"
        return
    fi

    sed -i "s|^listen:.*|listen: \"0.0.0.0:${new_port}\"|" "${CONFIG_FILE}"
    setup_firewall "${new_port}"
    echoContent green "端口已修改为：${new_port}"
    echoContent yellow "正在重启服务..."
    systemctl restart "${SERVICE_NAME}" 2>/dev/null || true
}

# 查看当前配置和分享链接
show_config_info() {
    if [[ ! -f "${CONFIG_FILE}" ]]; then
        echoContent red "错误：配置文件不存在，请先安装"
        return
    fi

    local is_standalone
    is_standalone=$(get_config_value "standalone")

    if [[ "${is_standalone}" == "true" ]]; then
        echoContent green "运行模式：独立模式"
        local password
        password=$(get_config_value "password")
        local listen_port
        listen_port=$(get_config_value "listen" | grep -oP ':\K[0-9]+')
        echoContent yellow "密码：${password}"
        echoContent yellow "端口：${listen_port}"
        show_share_link "${password}" "${listen_port}" ""
    else
        echoContent green "运行模式：Xboard 模式"
        echoContent yellow "面板地址：$(get_config_value 'api_host')"
        echoContent yellow "节点 ID：$(get_config_value 'node_id')"
        echoContent yellow "端口：$(get_config_value 'listen' | grep -oP ':\K[0-9]+')"
        echoContent yellow "密码由 Xboard 面板管理"
    fi
}

# ============================================================
# 服务管理函数
# ============================================================

# 启动服务
start_service() {
    if systemctl is-active "${SERVICE_NAME}" >/dev/null 2>&1; then
        echoContent yellow "AnytlsServer 已在运行中"
        return
    fi

    systemctl start "${SERVICE_NAME}"
    if systemctl is-active "${SERVICE_NAME}" >/dev/null 2>&1; then
        echoContent green "AnytlsServer 启动成功"
    else
        echoContent red "AnytlsServer 启动失败，请查看日志"
    fi
}

# 停止服务
stop_service() {
    if ! systemctl is-active "${SERVICE_NAME}" >/dev/null 2>&1; then
        echoContent yellow "AnytlsServer 未在运行"
        return
    fi

    systemctl stop "${SERVICE_NAME}"
    echoContent green "AnytlsServer 已停止"
}

# 重启服务
restart_service() {
    systemctl restart "${SERVICE_NAME}"
    if systemctl is-active "${SERVICE_NAME}" >/dev/null 2>&1; then
        echoContent green "AnytlsServer 重启成功"
    else
        echoContent red "AnytlsServer 重启失败，请查看日志"
    fi
}

# 查看服务状态
show_status() {
    systemctl status "${SERVICE_NAME}" --no-pager
}

# 查看日志
show_log() {
    echoContent yellow "按 Ctrl+C 退出日志查看"
    journalctl -u "${SERVICE_NAME}" -f --no-pager
}

# 更新版本
update_binary() {
    echoContent skyBlue "正在更新 AnytlsServer..."

    # 停止服务
    if systemctl is-active "${SERVICE_NAME}" >/dev/null 2>&1; then
        systemctl stop "${SERVICE_NAME}"
    fi

    # 下载新版本
    download_binary

    # 重启服务
    systemctl start "${SERVICE_NAME}"

    if systemctl is-active "${SERVICE_NAME}" >/dev/null 2>&1; then
        echoContent green "AnytlsServer 更新成功并已重启"
    else
        echoContent red "AnytlsServer 更新后启动失败，请查看日志"
    fi
}

# 卸载
uninstall() {
    echoContent skyBlue "正在卸载 AnytlsServer..."

    read -r -p "确认卸载？所有配置和数据将被删除 [y/N]: " confirm
    if [[ "${confirm}" != "y" && "${confirm}" != "Y" ]]; then
        echoContent yellow "取消卸载"
        return
    fi

    # 停止并禁用服务
    systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
    systemctl disable "${SERVICE_NAME}" 2>/dev/null || true

    # 删除文件
    rm -f "${SERVICE_FILE}"
    rm -f "${BINARY_PATH}"
    rm -rf "${CONFIG_DIR}"
    rm -rf "${LOG_DIR}"
    rm -f /tmp/anytls-traffic.json

    # 重新加载 systemd
    systemctl daemon-reload

    echoContent green "AnytlsServer 已完全卸载"
}

# ============================================================
# 安装主流程
# ============================================================

# 完整安装流程
install() {
    echoContent skyBlue "\n开始安装 AnytlsServer..."
    echoContent red "=============================================================="

    # 1. 系统检测
    check_system
    check_arch

    # 2. 安装依赖
    install_dependencies

    # 3. 下载二进制文件
    download_binary

    # 4. 创建必要目录
    mkdir -p "${CONFIG_DIR}"
    mkdir -p "${LOG_DIR}"

    # 5. 配置 systemd 服务
    setup_systemd

    # 6. 交互式配置
    setup_config

    # 7. 启动服务
    echoContent skyBlue "\n正在启动 AnytlsServer..."
    systemctl start "${SERVICE_NAME}"

    if systemctl is-active "${SERVICE_NAME}" >/dev/null 2>&1; then
        echoContent green "\nAnytlsServer 安装完成并已启动！"
    else
        echoContent yellow "\nAnytlsServer 安装完成，但启动失败"
        echoContent yellow "请使用 journalctl -u ${SERVICE_NAME} 查看日志"
    fi

    echoContent red "=============================================================="
}

# ============================================================
# 交互式管理菜单
# ============================================================

# 显示菜单
show_menu() {
    echoContent red "\n=============================================================="
    echoContent green "AnytlsServer 一键安装管理脚本 ${SCRIPT_VERSION}"
    echoContent green "Github: https://github.com/${GITHUB_REPO}"
    echoContent red "=============================================================="

    # 显示当前状态
    if [[ -f "${BINARY_PATH}" ]]; then
        if systemctl is-active "${SERVICE_NAME}" >/dev/null 2>&1; then
            echoContent green "当前状态：运行中"
        else
            echoContent yellow "当前状态：已停止"
        fi
    else
        echoContent yellow "当前状态：未安装"
    fi

    echoContent red "=============================================================="
    echoContent skyBlue "------------------------安装管理------------------------------"
    echoContent yellow "1. 安装 AnytlsServer"
    echoContent yellow "2. 更新 AnytlsServer"
    echoContent yellow "3. 卸载 AnytlsServer"
    echoContent skyBlue "------------------------服务管理------------------------------"
    echoContent yellow "4. 启动服务"
    echoContent yellow "5. 停止服务"
    echoContent yellow "6. 重启服务"
    echoContent yellow "7. 查看状态"
    echoContent yellow "8. 查看日志"
    echoContent skyBlue "------------------------配置管理------------------------------"
    echoContent yellow "9. 修改配置"
    echoContent yellow "10. 查看配置/分享链接"
    echoContent yellow "11. 证书续期"
    echoContent red "=============================================================="
    echoContent yellow "0. 退出"
    echoContent red "=============================================================="

    read -r -p "请选择: " menu_choice

    case ${menu_choice} in
    1)
        install
        ;;
    2)
        check_arch
        update_binary
        ;;
    3)
        uninstall
        ;;
    4)
        start_service
        ;;
    5)
        stop_service
        ;;
    6)
        restart_service
        ;;
    7)
        show_status
        ;;
    8)
        show_log
        ;;
    9)
        modify_config
        ;;
    10)
        show_config_info
        ;;
    11)
        renew_cert
        ;;
    0)
        exit 0
        ;;
    *)
        echoContent red "无效选择"
        ;;
    esac
}

# ============================================================
# 脚本入口
# ============================================================

# 检查 root 权限
check_root

# 根据参数决定执行模式
case "${1:-}" in
"install")
    # 直接执行安装
    check_system
    check_arch
    install
    ;;
*)
    # 显示交互式菜单
    show_menu
    ;;
esac
