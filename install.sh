#!/bin/sh
set -e

#================================================================
# AnyTLS Server 一键安装脚本
# 功能：自动检测架构、下载二进制、配置 systemd/OpenRC 服务
# 用法：sh install.sh --apiHost=xxx --apiKey=yyy --nodeID=zzz
#================================================================

# 全局变量
APIHOST=""
APIKEY=""
NODEID=""
ANYTLS_ARCH=""
SYSTEM_TYPE=""
DOWNLOAD_URL=""
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="anytls-server"
SERVICE_NAME="anytls"
GITHUB_REPO="code-gopher/anytls-go"

#----------------------------------------------------------------
# 函数：显示用法
#----------------------------------------------------------------
show_usage() {
    echo "用法: $0 --apiHost=xxx --apiKey=yyy --nodeID=zzz"
    exit 1
}

#----------------------------------------------------------------
# 函数：解析命令行参数
#----------------------------------------------------------------
parse_arguments() {
    for arg in "$@"; do
        case $arg in
            --apiHost=*)
                APIHOST="${arg#*=}"
                ;;
            --apiKey=*)
                APIKEY="${arg#*=}"
                ;;
            --nodeID=*)
                NODEID="${arg#*=}"
                ;;
            *)
                echo "错误：未知参数 $arg"
                show_usage
                ;;
        esac
    done

    # 校验必填参数
    if [ -z "$APIHOST" ] || [ -z "$APIKEY" ] || [ -z "$NODEID" ]; then
        echo "错误：必须提供 --apiHost、--apiKey、--nodeID 三个参数"
        show_usage
    fi

    echo "==> 配置参数："
    echo "    apiHost = ${APIHOST}"
    echo "    apiKey  = ${APIKEY}"
    echo "    nodeID  = ${NODEID}"
    echo ""
}

#----------------------------------------------------------------
# 函数：检测系统架构
#----------------------------------------------------------------
detect_architecture() {
    echo "==> 检测系统架构..."
    local arch=$(uname -m)

    case $arch in
        x86_64|amd64)
            ANYTLS_ARCH="amd64"
            echo "    架构：64 位 x86 (amd64)"
            ;;
        aarch64|arm64)
            ANYTLS_ARCH="arm64"
            echo "    架构：64 位 ARM (arm64)"
            ;;
        *)
            echo "    错误：不支持的架构 $arch"
            exit 1
            ;;
    esac
    echo ""
}

#----------------------------------------------------------------
# 函数：检测系统服务管理器
#----------------------------------------------------------------
detect_system_type() {
    echo "==> 检测系统服务管理器..."

    if command -v systemctl >/dev/null 2>&1; then
        SYSTEM_TYPE="systemd"
        echo "    类型：systemd"
    elif command -v rc-service >/dev/null 2>&1; then
        SYSTEM_TYPE="openrc"
        echo "    类型：OpenRC"
    else
        echo "    错误：不支持的服务管理器（需要 systemd 或 OpenRC）"
        exit 1
    fi
    echo ""
}

#----------------------------------------------------------------
# 函数：安装必要依赖
#----------------------------------------------------------------
install_dependencies() {
    echo "==> 检查并安装必要依赖..."

    if [ "$SYSTEM_TYPE" = "openrc" ]; then
        apk add --no-cache wget
    else
        if command -v apt-get >/dev/null 2>&1; then
            apt-get install -y wget
        elif command -v yum >/dev/null 2>&1; then
            yum install -y wget
        else
            echo "    警告：无法自动安装 wget，请手动安装后重试"
        fi
    fi
    echo "    依赖安装完成"
    echo ""
}

#----------------------------------------------------------------
# 函数：获取最新版本号
#----------------------------------------------------------------
get_latest_version() {
    echo "==> 获取最新版本号..."
    LATEST_VERSION=$(wget -qO- "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
        | grep '"tag_name"' \
        | head -1 \
        | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

    if [ -z "$LATEST_VERSION" ]; then
        echo "    警告：无法获取最新版本，使用默认版本 v0.0.13"
        LATEST_VERSION="v0.0.13"
    fi
    echo "    最新版本：${LATEST_VERSION}"
    echo ""
}

#----------------------------------------------------------------
# 函数：下载 AnyTLS 二进制
#----------------------------------------------------------------
download_binary() {
    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_VERSION}/anytls-server-linux-${ANYTLS_ARCH}"

    echo "==> 下载 AnyTLS Server 二进制..."
    echo "    URL: ${DOWNLOAD_URL}"

    rm -f /tmp/anytls-server-download
    if ! wget -O /tmp/anytls-server-download "$DOWNLOAD_URL"; then
        echo "    错误：下载失败"
        exit 1
    fi

    echo "    下载完成"
    echo ""
}

#----------------------------------------------------------------
# 函数：安装二进制文件
#----------------------------------------------------------------
install_binary() {
    echo "==> 安装 AnyTLS Server 二进制..."

    cp -f /tmp/anytls-server-download "${INSTALL_DIR}/${BINARY_NAME}"
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    rm -f /tmp/anytls-server-download

    echo "    安装路径：${INSTALL_DIR}/${BINARY_NAME}"
    echo "    版本：$("${INSTALL_DIR}/${BINARY_NAME}" --help 2>&1 | head -1 || echo '未知')"
    echo ""
}

#----------------------------------------------------------------
# 函数：配置 systemd 服务
#----------------------------------------------------------------
configure_systemd_service() {
    echo "==> 配置 systemd 服务..."

    cat > /etc/systemd/system/${SERVICE_NAME}.service <<EOF
[Unit]
Description=AnyTLS Server
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} \\
    --v2board-api-host ${APIHOST} \\
    --v2board-api-key ${APIKEY} \\
    --v2board-node-id ${NODEID}
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME}
    systemctl restart ${SERVICE_NAME}

    echo "    systemd 服务配置完成并已启动"
    echo ""
}

#----------------------------------------------------------------
# 函数：配置 OpenRC 服务
#----------------------------------------------------------------
configure_openrc_service() {
    echo "==> 配置 OpenRC 服务..."

    cat > /etc/init.d/${SERVICE_NAME} <<EOF
#!/sbin/openrc-run

name="${SERVICE_NAME}"
description="AnyTLS Server"
command="${INSTALL_DIR}/${BINARY_NAME}"
command_args="--v2board-api-host ${APIHOST} --v2board-api-key ${APIKEY} --v2board-node-id ${NODEID}"
pidfile="/var/run/${SERVICE_NAME}.pid"
command_background="yes"

depend() {
    need net
}
EOF

    chmod +x /etc/init.d/${SERVICE_NAME}
    rc-update add ${SERVICE_NAME} default
    rc-service ${SERVICE_NAME} restart

    echo "    OpenRC 服务配置完成并已启动"
    echo ""
}

#----------------------------------------------------------------
# 函数：配置服务（根据系统类型分发）
#----------------------------------------------------------------
configure_service() {
    if [ "$SYSTEM_TYPE" = "systemd" ]; then
        configure_systemd_service
    else
        configure_openrc_service
    fi
}

#----------------------------------------------------------------
# 函数：显示完成信息
#----------------------------------------------------------------
show_completion_message() {
    echo "================================================"
    echo "✓ AnyTLS Server 安装完成！"
    echo "================================================"
    echo ""
    echo "服务管理命令："
    if [ "$SYSTEM_TYPE" = "systemd" ]; then
        echo "  查看状态：systemctl status ${SERVICE_NAME}"
        echo "  查看日志：journalctl -u ${SERVICE_NAME} -f"
        echo "  重启服务：systemctl restart ${SERVICE_NAME}"
        echo "  停止服务：systemctl stop ${SERVICE_NAME}"
    else
        echo "  查看状态：rc-service ${SERVICE_NAME} status"
        echo "  重启服务：rc-service ${SERVICE_NAME} restart"
        echo "  停止服务：rc-service ${SERVICE_NAME} stop"
    fi
    echo ""
}

#----------------------------------------------------------------
# 主函数
#----------------------------------------------------------------
main() {
    echo ""
    echo "================================================"
    echo "  AnyTLS Server 一键安装脚本"
    echo "================================================"
    echo ""

    # 检查 root 权限
    if [ "$(id -u)" -ne 0 ]; then
        echo "错误：请使用 root 权限运行此脚本"
        exit 1
    fi

    parse_arguments "$@"
    detect_architecture
    detect_system_type
    install_dependencies
    get_latest_version
    download_binary
    install_binary
    configure_service
    show_completion_message
}

# 执行主函数
main "$@"
