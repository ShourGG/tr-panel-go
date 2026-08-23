#!/bin/bash

#                        _oo0oo_
#                       o8888888o
#                       88" . "88
#                       (| -_- |)
#                       0\  =  /0
#                     ___/`---'\___
#                   .' \\|     |// '.
#                  / \\|||  :  |||// \
#                 / _||||| -:- |||||- \
#                |   | \\\  - /// |   |
#                | \_|  ''\---/''  |_/ |
#                \  .-\__  '-'  ___/-. /
#              ___'. .'  /--.--\  `. .'___
#           ."" '<  `.___\_<|>_/___.' >' "".
#          | | :  `- \`.;`\ _ /`;.`/ - ` : | |
#          \  \ `_.   \_ __\ /__ _/   .-` /  /
#      =====`-.____`.___ \_____/___.-`___.-'=====
#                        `=---='
# 
# 
#      ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
# 
#            佛祖保佑       永不宕机     永无BUG
# 
#        佛曰:  
#                写字楼里写字间，写字间里程序员；  
#                程序人员写程序，又拿程序换酒钱。  
#                酒醒只在网上坐，酒醉还来网下眠；  
#                酒醉酒醒日复日，网上网下年复年。  
#                但愿老死电脑间，不愿鞠躬老板前；  
#                奔驰宝马贵者趣，公交自行程序员。  
#                别人笑我忒疯癫，我笑自己命太贱；  
#                不见满街漂亮妹，哪个归得程序员？

set -e

# 脚本版本
SCRIPT_VERSION="1.5.1"

# 定义变量
INSTALL_DIR="/opt/tr-panel"
SERVICE_NAME="tr-panel"
PORT=8800
UPDATE_CHANNEL="stable"
CHANNEL_FILE="${INSTALL_DIR}/update-channel"

# ────────── GitHub 镜像支持 ──────────
# 镜像列表：名称|API前缀|下载前缀|Raw前缀
MIRRORS=(
    "gh-proxy.com (推荐)|https://gh-proxy.com/https://api.github.com|https://gh-proxy.com/https://github.com|https://gh-proxy.com/https://raw.githubusercontent.com"
    "xs.shour.ccwu.cc:5678|http://xs.shour.ccwu.cc:5678/https://api.github.com|http://xs.shour.ccwu.cc:5678/https://github.com|http://xs.shour.ccwu.cc:5678/https://raw.githubusercontent.com"
    "GitHub 官方 (直连)|https://api.github.com|https://github.com|https://raw.githubusercontent.com"
)

# 当前选中的镜像索引（默认 0 = gh-proxy.com）
MIRROR_IDX=0
VERSION_OVERRIDE=""
BACKUP_BINARY=""
BACKUP_SERVICE=""
DOWNLOAD_TIMEOUT=180
DOWNLOAD_RETRIES=3

get_mirror_api()      { echo "${MIRRORS[$MIRROR_IDX]}" | cut -d'|' -f2; }
get_mirror_download() { echo "${MIRRORS[$MIRROR_IDX]}" | cut -d'|' -f3; }
get_mirror_raw()      { echo "${MIRRORS[$MIRROR_IDX]}" | cut -d'|' -f4; }

normalize_update_channel() {
    local channel=$(echo "$1" | tr '[:upper:]' '[:lower:]' | xargs)
    if [ "$channel" = "dev" ]; then
        echo "dev"
    else
        echo "stable"
    fi
}

load_update_channel() {
    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"
    local detected=""

    if [ -f "$service_file" ]; then
        detected=$(grep '^Environment="UPDATE_CHANNEL=' "$service_file" 2>/dev/null | tail -1 | cut -d'"' -f2 | cut -d'=' -f2)
    fi

    if [ -z "$detected" ] && [ -f "$CHANNEL_FILE" ]; then
        detected=$(cat "$CHANNEL_FILE" 2>/dev/null)
    fi

    UPDATE_CHANNEL=$(normalize_update_channel "$detected")
}

save_update_channel() {
    mkdir -p "$INSTALL_DIR"
    echo "$UPDATE_CHANNEL" > "$CHANNEL_FILE"

    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"
    if [ -f "$service_file" ]; then
        if grep -q '^Environment="UPDATE_CHANNEL=' "$service_file"; then
            sed -i "s/^Environment=\"UPDATE_CHANNEL=.*/Environment=\"UPDATE_CHANNEL=${UPDATE_CHANNEL}\"/" "$service_file"
        else
            sed -i "/^Environment=\"JWT_SECRET=/a Environment=\"UPDATE_CHANNEL=${UPDATE_CHANNEL}\"" "$service_file"
        fi

        systemctl daemon-reload
        if systemctl list-unit-files | grep -q "^${SERVICE_NAME}\.service"; then
            systemctl restart "$SERVICE_NAME" >/dev/null 2>&1 || true
        fi
    fi
}

select_update_channel() {
    check_root
    load_update_channel

    echo ""
    echo -e "${BLUE}========== 切换更新通道 ==========${NC}"
    if [ "$UPDATE_CHANNEL" = "dev" ]; then
        echo -e "  ${GREEN}[1] 开发板 dev  (当前)${NC}"
        echo "  [0] 正式版 stable"
    else
        echo -e "  ${GREEN}[0] 正式版 stable  (当前)${NC}"
        echo "  [1] 开发板 dev"
    fi
    echo ""
    read -p "请选择更新通道 [0-1]（回车保持当前）: " channel_idx

    if [ -z "$channel_idx" ]; then
        return
    fi

    case "$channel_idx" in
        0) UPDATE_CHANNEL="stable" ;;
        1) UPDATE_CHANNEL="dev" ;;
        *)
            echo -e "${RED}无效选择${NC}"
            return
            ;;
    esac

    save_update_channel
    echo -e "${GREEN}更新通道已切换为: ${UPDATE_CHANNEL}${NC}"
    if [ "$UPDATE_CHANNEL" = "dev" ]; then
        echo -e "${YELLOW}当前机器将只跟踪开发预发布版本${NC}"
    else
        echo -e "${YELLOW}当前机器将只跟踪正式稳定版本${NC}"
    fi
    echo ""
}

select_mirror() {
    echo ""
    echo -e "${BLUE}========== 选择 GitHub 镜像 ==========${NC}"
    echo -e "${YELLOW}如果下载速度慢或无法连接 GitHub，请选择镜像加速${NC}"
    echo ""
    local i=0
    for m in "${MIRRORS[@]}"; do
        local name=$(echo "$m" | cut -d'|' -f1)
        if [ $i -eq $MIRROR_IDX ]; then
            echo -e "  ${GREEN}[$i] $name  (当前)${NC}"
        else
            echo "  [$i] $name"
        fi
        i=$((i+1))
    done
    echo ""
    read -p "请选择镜像 [0-$((${#MIRRORS[@]}-1))]（回车保持当前）: " idx
    if [ -n "$idx" ] && [ "$idx" -ge 0 ] 2>/dev/null && [ "$idx" -lt "${#MIRRORS[@]}" ] 2>/dev/null; then
        MIRROR_IDX=$idx
        local name=$(echo "${MIRRORS[$MIRROR_IDX]}" | cut -d'|' -f1)
        echo -e "${GREEN}已切换到: $name${NC}"
    fi
    echo ""
}

# 从 GitHub API 获取最新版本号（失败则报错退出，不使用写死的兜底版本）
get_latest_version() {
    local api_base=$(get_mirror_api)
    load_update_channel

    if [ -n "$VERSION_OVERRIDE" ]; then
        echo "$VERSION_OVERRIDE"
        return 0
    fi

    if [ "$UPDATE_CHANNEL" = "dev" ]; then
        local response
        response=$(timeout 15 curl -s --connect-timeout 5 --max-time 15 \
            "${api_base}/repos/ShourGG/tr-panel-go/releases?per_page=20" 2>/dev/null)
        if command -v python3 >/dev/null 2>&1; then
            LATEST=$(printf '%s' "$response" | python3 -c "import json, sys
data = json.load(sys.stdin)
for item in data:
    if not item.get('draft') and item.get('prerelease'):
        print(item.get('tag_name', ''))
        break
")
        else
            LATEST=$(echo "$response" | grep -o '"tag_name":"[^"]*".*"prerelease":true' | head -1 | cut -d'"' -f4)
        fi
    else
        LATEST=$(timeout 15 curl -s --connect-timeout 5 --max-time 15 \
            "${api_base}/repos/ShourGG/tr-panel-go/releases/latest" \
            2>/dev/null | grep '"tag_name"' | head -1 | cut -d'"' -f4)
    fi
    if [ -z "$LATEST" ]; then
        echo ""
        return 1
    else
        echo "$LATEST"
        return 0
    fi
}

print_missing_channel_release_error() {
    if [ "$UPDATE_CHANNEL" = "dev" ]; then
        echo -e "${RED}错误: 当前 dev 通道没有可用的开发预发布版本${NC}"
        echo -e "${YELLOW}请先发布一个 prerelease，例如 v1.3.18-dev.1${NC}"
        echo -e "${YELLOW}或者先切回 stable 通道再更新面板${NC}"
    else
        echo -e "${RED}错误: 无法获取 stable 通道的正式版本，请检查网络连接或切换镜像 [选项 12]${NC}"
    fi
}

# 构建下载 URL
get_release_download_url() {
    local version="$1"
    local filename="$2"
    local dl_base=$(get_mirror_download)
    echo "${dl_base}/ShourGG/tr-panel-go/releases/download/${version}/${filename}"
}

# 构建 raw 文件 URL
get_raw_url() {
    local path="$1"
    local raw_base=$(get_mirror_raw)
    echo "${raw_base}/ShourGG/tr-panel-go/main/${path}"
}

# 下载文件时统一设置超时和重试，避免网络异常让一键安装永久卡住。
download_file() {
    local url="$1"
    local output="$2"
    rm -f "$output"

    if command -v curl >/dev/null 2>&1; then
        curl -fL --retry "$DOWNLOAD_RETRIES" --retry-delay 2 --retry-all-errors \
            --connect-timeout 10 --max-time "$DOWNLOAD_TIMEOUT" -o "$output" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget --tries="$DOWNLOAD_RETRIES" --timeout=30 --dns-timeout=10 \
            --connect-timeout=10 --read-timeout=30 -O "$output" "$url"
    else
        echo -e "${RED}错误: 需要安装 wget 或 curl${NC}"
        return 1
    fi

    [ -s "$output" ]
}

# 当前镜像失败时自动尝试其它已配置镜像，并校验 release 资产。
download_release_binary() {
    local version="$1"
    local output="$2"
    local original_idx="$MIRROR_IDX"
    local downloaded=0
    local downloaded_idx="$original_idx"
    local idx
    local url

    for idx in "$original_idx" $(seq 0 $((${#MIRRORS[@]} - 1))); do
        if [ "$idx" = "$original_idx" ] && [ "$downloaded" -ne 0 ]; then
            continue
        fi
        MIRROR_IDX="$idx"
        url=$(get_release_download_url "$version" "terraria-panel")
        echo -e "${BLUE}下载地址: ${url}${NC}"
        if download_file "$url" "$output"; then
            downloaded=1
            downloaded_idx="$idx"
            break
        fi
        echo -e "${YELLOW}当前下载源失败，尝试下一个下载源...${NC}"
    done
    if [ "$downloaded" -ne 1 ] || [ ! -s "$output" ]; then
        MIRROR_IDX="$original_idx"
        echo -e "${RED}错误: 所有下载源均无法获取 TR Panel ${version}${NC}"
        rm -f "$output"
        return 1
    fi

    local checksum_file="${output}.sha256"
    local checksum_url
    MIRROR_IDX="$downloaded_idx"
    checksum_url=$(get_release_download_url "$version" "SHA256SUMS")
    if download_file "$checksum_url" "$checksum_file" && command -v sha256sum >/dev/null 2>&1; then
        local expected actual
        expected=$(awk '$2 == "terraria-panel" {print $1; exit}' "$checksum_file")
        actual=$(sha256sum "$output" | awk '{print $1}')
        if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
            MIRROR_IDX="$original_idx"
            echo -e "${RED}错误: TR Panel SHA-256 校验失败${NC}"
            rm -f "$output" "$checksum_file"
            return 1
        fi
        echo -e "${GREEN}SHA-256 校验通过: ${actual}${NC}"
    else
        echo -e "${YELLOW}警告: 无法下载 SHA256SUMS，继续使用已下载的二进制${NC}"
    fi
    rm -f "$checksum_file"
    MIRROR_IDX="$original_idx"
}

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 检查是否为root用户
check_root() {
    if [ "$EUID" -ne 0 ]; then 
        echo -e "${RED}错误: 请使用 root 用户或 sudo 运行此脚本${NC}"
        exit 1
    fi
}

validate_port() {
    if ! [[ "$PORT" =~ ^[0-9]+$ ]] || [ "$PORT" -lt 1 ] || [ "$PORT" -gt 65535 ]; then
        echo -e "${RED}错误: 端口必须是 1-65535 之间的数字${NC}"
        exit 2
    fi
}

rollback_install() {
    echo -e "${RED}安装后的健康检查失败，开始恢复旧版本...${NC}"
    if [ -n "$BACKUP_BINARY" ] && [ -f "$BACKUP_BINARY" ]; then
        cp -f "$BACKUP_BINARY" "$INSTALL_DIR/tr-panel"
        chmod +x "$INSTALL_DIR/tr-panel"
    fi
    if [ -n "$BACKUP_SERVICE" ] && [ -f "$BACKUP_SERVICE" ]; then
        cp -f "$BACKUP_SERVICE" "/etc/systemd/system/${SERVICE_NAME}.service"
    fi
    systemctl daemon-reload 2>/dev/null || true
    systemctl restart "$SERVICE_NAME" 2>/dev/null || true
}

# 版本比较函数
version_gt() {
    test "$(printf '%s\n' "$@" | sort -V | head -n 1)" != "$1"
}

# 检查脚本更新
check_script_update() {
    # 获取远程版本号（超时1秒）
    REMOTE_VERSION=$(timeout 1 curl -s --connect-timeout 1 --max-time 1 "$(get_raw_url 'tr.sh')" 2>/dev/null | grep "^SCRIPT_VERSION=" | head -1 | cut -d'"' -f2)
    
    if [ -z "$REMOTE_VERSION" ]; then
        return
    fi
    
    # 比较版本
    if version_gt "$REMOTE_VERSION" "$SCRIPT_VERSION"; then
        echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e "${YELLOW}  发现新版本${NC}"
        echo -e "${YELLOW}  当前版本: ${SCRIPT_VERSION}${NC}"
        echo -e "${YELLOW}  最新版本: ${REMOTE_VERSION}${NC}"
        echo -e "${YELLOW}  建议选择 [6] 更新脚本${NC}"
        echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo ""
    fi
}

# 显示菜单
show_menu() {
    clear
    load_update_channel
    
    # 检查服务状态
    if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
        STATUS="${GREEN}[运行中]${NC}"
    else
        STATUS="${RED}[已停止]${NC}"
    fi
    
    echo "========================================="
    echo "  TR Panel 管理脚本 v${SCRIPT_VERSION}"
    echo "  https://github.com/ShourGG/tr-panel-go"
    echo "========================================="
    echo -e "服务状态: $STATUS"
    echo ""
    
    # 检查脚本更新
    check_script_update
    
    echo -e "${YELLOW}系统要求: Ubuntu 24+ (低版本可能出现 GLIBC 版本报错)${NC}"
    echo -e "${BLUE}更新通道: ${UPDATE_CHANNEL}${NC}"
    echo ""
    echo "————————————————————————————————————————"
    echo "[0]: 下载并启动服务 (Download and start)"
    echo "————————————————————————————————————————"
    echo "[1]: 启动服务 (Start service)"
    echo "[2]: 停止服务 (Stop service)"
    echo "[3]: 重启服务 (Restart service)"
    echo "————————————————————————————————————————"
    echo "[4]: 更新面板 (Update panel)"
    echo "[5]: 强制更新 (Force update)"
    echo "[6]: 更新脚本 (Update script)"
    echo "————————————————————————————————————————"
    echo "[7]: 查看状态 (View status)"
    echo "[8]: 查看日志 (View logs)"
    echo "[9]: 修改端口 (Change port)"
    echo "[10]: 卸载面板 (Uninstall)"
    echo "————————————————————————————————————————"
    local mirror_name=$(echo "${MIRRORS[$MIRROR_IDX]}" | cut -d'|' -f1)
    echo -e "[12]: 切换 GitHub 镜像 (当前: ${GREEN}${mirror_name}${NC})"
    echo -e "[13]: 切换更新通道 (当前: ${GREEN}${UPDATE_CHANNEL}${NC})"
    echo "[11]: 退出脚本 (Exit)"
    echo "————————————————————————————————————————"
    echo ""
}

# 下载并启动
install_service() {
    check_root
    validate_port
    load_update_channel
    echo -e "${GREEN}[1/6] 创建安装目录并保留现有数据...${NC}"
    mkdir -p "$INSTALL_DIR" "$INSTALL_DIR/rollback"
    cd "$INSTALL_DIR"

    VERSION=$(get_latest_version || true)
    if [ -z "$VERSION" ]; then
        print_missing_channel_release_error
        exit 1
    fi
    echo -e "${GREEN}[2/6] 下载 TR Panel ${VERSION}...${NC}"
    if ! download_release_binary "$VERSION" tr-panel.new; then
        exit 1
    fi

    echo -e "${GREEN}[3/6] 备份旧二进制并原子替换...${NC}"
    if [ -f tr-panel ]; then
        BACKUP_BINARY="$INSTALL_DIR/rollback/tr-panel.$(date +%Y%m%d_%H%M%S).bak"
        cp -f tr-panel "$BACKUP_BINARY"
    else
        BACKUP_BINARY=""
    fi
    if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
        BACKUP_SERVICE="$INSTALL_DIR/rollback/${SERVICE_NAME}.$(date +%Y%m%d_%H%M%S).service.bak"
        cp -f "/etc/systemd/system/${SERVICE_NAME}.service" "$BACKUP_SERVICE"
    else
        BACKUP_SERVICE=""
    fi
    chmod +x tr-panel.new
    mv -f tr-panel.new tr-panel

    echo -e "${GREEN}[4/6] 创建 systemd 服务...${NC}"
    # 生成随机 JWT_SECRET（若服务文件已存在则复用旧密钥）
    EXISTING_SECRET=$(grep 'JWT_SECRET=' /etc/systemd/system/${SERVICE_NAME}.service 2>/dev/null | cut -d'"' -f2 | cut -d'=' -f2)
    if [ -n "$EXISTING_SECRET" ]; then
        JWT_SECRET="$EXISTING_SECRET"
    else
        JWT_SECRET=$(openssl rand -hex 32 2>/dev/null || cat /proc/sys/kernel/random/uuid | tr -d '-' | head -c 64)
    fi
    cat > /etc/systemd/system/${SERVICE_NAME}.service <<EOF
[Unit]
Description=TR Panel Go Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/tr-panel
Restart=always
RestartSec=10
Environment="PORT=$PORT"
Environment="JWT_SECRET=$JWT_SECRET"
Environment="UPDATE_CHANNEL=$UPDATE_CHANNEL"

[Install]
WantedBy=multi-user.target
EOF
    echo "$UPDATE_CHANNEL" > "$CHANNEL_FILE"

    echo -e "${GREEN}[5/6] 启动服务...${NC}"
    systemctl daemon-reload
    systemctl enable $SERVICE_NAME
    systemctl restart $SERVICE_NAME 2>/dev/null || systemctl start $SERVICE_NAME

    echo -e "${GREEN}[6/6] 等待服务健康...${NC}"
    HEALTHY=0
    for _ in $(seq 1 20); do
        if systemctl is-active --quiet "$SERVICE_NAME" && curl -fsS --max-time 2 "http://127.0.0.1:${PORT}/" >/dev/null 2>&1; then
            HEALTHY=1
            break
        fi
        sleep 1
    done
    if [ "$HEALTHY" -ne 1 ]; then
        rollback_install
        echo -e "${RED}错误: 面板服务启动失败，查看 journalctl -u ${SERVICE_NAME}${NC}"
        exit 1
    fi

    echo ""
    echo -e "${GREEN}=========================================${NC}"
    echo -e "${GREEN}  安装完成${NC}"
    echo -e "${GREEN}=========================================${NC}"
    echo ""
    PUBLIC_IP=$(timeout 3 curl -s --max-time 3 ifconfig.me 2>/dev/null || timeout 3 curl -s --max-time 3 ip.sb 2>/dev/null)
    LOCAL_IP=$(hostname -I | awk '{print $1}')
    echo -e "内网访问: ${GREEN}http://${LOCAL_IP}:${PORT}${NC}"
    if [ -n "$PUBLIC_IP" ]; then
        echo -e "公网访问: ${GREEN}http://${PUBLIC_IP}:${PORT}${NC}"
    fi
    echo ""
}

# 启动服务
start_service() {
    check_root
    systemctl start $SERVICE_NAME
    echo -e "${GREEN}服务已启动${NC}"
}

# 停止服务
stop_service() {
    check_root
    systemctl stop $SERVICE_NAME
    echo -e "${YELLOW}服务已停止${NC}"
}

# 重启服务
restart_service() {
    check_root
    systemctl restart $SERVICE_NAME
    echo -e "${GREEN}服务已重启${NC}"
}

# 更新面板
update_panel() {
    check_root
    load_update_channel
    echo -e "${BLUE}当前更新通道: ${UPDATE_CHANNEL}${NC}"
    VERSION=$(get_latest_version || true)
    if [ -z "$VERSION" ]; then
        print_missing_channel_release_error
        exit 1
    fi
    echo -e "${GREEN}开始更新面板 ${VERSION}...${NC}"
    
    systemctl stop $SERVICE_NAME
    cd $INSTALL_DIR
    
    # 备份旧二进制，防止下载失败后无法回滚
    if [ -f tr-panel ]; then
        cp tr-panel tr-panel.bak
        echo -e "${GREEN}已备份旧版本为 tr-panel.bak${NC}"
    fi
    
    # 下载新版本（失败时自动回滚）
    if download_release_binary "$VERSION" tr-panel.new; then
        mv tr-panel.new tr-panel
        chmod +x tr-panel
        rm -f tr-panel.bak
        systemctl start $SERVICE_NAME
        echo -e "${GREEN}更新完成，当前版本: ${VERSION}${NC}"
    else
        # 回滚
        rm -f tr-panel.new tr-panel.new.sha256
        if [ -f tr-panel.bak ]; then
            mv tr-panel.bak tr-panel
            echo -e "${RED}下载失败，已自动回滚到旧版本，服务恢复运行${NC}"
            systemctl start $SERVICE_NAME
        else
            echo -e "${RED}下载失败且无备份可回滚，请手动重新执行 [0] 安装${NC}"
        fi
    fi
}

# 强制更新
force_update() {
    check_root
    echo -e "${YELLOW}强制更新面板...${NC}"
    # 保留 data、rooms、servers、backups 和 JWT_SECRET；安装流程会原子替换二进制。
    install_service
}

# 更新脚本
update_script() {
    echo -e "${GREEN}更新脚本...${NC}"
    SCRIPT_URL="$(get_raw_url 'tr.sh')?nocache=$(date +%s)"
    echo -e "${BLUE}下载地址: ${SCRIPT_URL}${NC}"
    SCRIPT_PATH=$(realpath "$0")
    SCRIPT_TMP="${SCRIPT_PATH}.new"
    if download_file "$SCRIPT_URL" "$SCRIPT_TMP"; then
        chmod +x "$SCRIPT_TMP"
        mv -f "$SCRIPT_TMP" "$SCRIPT_PATH"
    else
        rm -f "$SCRIPT_TMP"
        echo -e "${RED}错误: 脚本下载失败，保留当前脚本${NC}"
        exit 1
    fi
    echo -e "${GREEN}脚本已更新至最新版本，请重新运行: $SCRIPT_PATH${NC}"
    exit 0
}

# 查看状态
view_status() {
    systemctl status $SERVICE_NAME
}

# 查看日志
view_logs() {
    journalctl -u $SERVICE_NAME -f
}

# 修改端口
change_port() {
    check_root
    echo -e "${YELLOW}当前端口: ${PORT}${NC}"
    echo ""
    read -p "请输入新端口 (1-65535): " NEW_PORT
    
    if ! [[ "$NEW_PORT" =~ ^[0-9]+$ ]] || [ "$NEW_PORT" -lt 1 ] || [ "$NEW_PORT" -gt 65535 ]; then
        echo -e "${RED}错误: 端口必须是 1-65535 之间的数字${NC}"
        return
    fi
    
    # 更新systemd服务配置
    sed -i "s/Environment=\"PORT=[0-9]*\"/Environment=\"PORT=${NEW_PORT}\"/" /etc/systemd/system/${SERVICE_NAME}.service
    
    # 更新脚本中的默认端口
    sed -i "s/^PORT=.*/PORT=${NEW_PORT}/" "$0"
    
    # 重启服务
    systemctl daemon-reload
    systemctl restart $SERVICE_NAME
    
    echo -e "${GREEN}端口已修改为: ${NEW_PORT}${NC}"
    echo -e "访问地址: ${GREEN}http://$(hostname -I | awk '{print $1}'):${NEW_PORT}${NC}"
    
    PORT=$NEW_PORT
}

# 卸载面板
uninstall() {
    check_root
    echo -e "${RED}确认卸载 TR Panel? (y/n)${NC}"
    read -r confirm
    if [ "$confirm" = "y" ]; then
        systemctl stop $SERVICE_NAME
        systemctl disable $SERVICE_NAME
        rm -f /etc/systemd/system/${SERVICE_NAME}.service
        rm -rf $INSTALL_DIR
        systemctl daemon-reload
        echo -e "${GREEN}卸载完成${NC}"
    fi
}

# 非交互安装：wget/curl 后直接执行 ./tr.sh --install --port 8800
if [ "${1:-}" = "--install" ] || [ "${TR_PANEL_AUTO_INSTALL:-}" = "1" ]; then
    shift || true
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --port)
                if [ "$#" -lt 2 ]; then
                    echo -e "${RED}错误: --port 需要一个端口值${NC}"
                    exit 2
                fi
                PORT="$2"
                shift 2
                ;;
            --port=*)
                PORT="${1#*=}"
                shift
                ;;
            --version)
                if [ "$#" -lt 2 ]; then
                    echo -e "${RED}错误: --version 需要一个版本标签${NC}"
                    exit 2
                fi
                VERSION_OVERRIDE="$2"
                shift 2
                ;;
            --version=*)
                VERSION_OVERRIDE="${1#*=}"
                shift
                ;;
            --mirror=*)
                MIRROR_IDX="${1#*=}"
                shift
                ;;
            *)
                echo -e "${RED}未知参数: $1${NC}"
                exit 2
                ;;
        esac
    done
    install_service
    exit 0
fi

# 主循环
while true; do
    show_menu
    read -p "请输入选择 (Please enter your selection) [0-13]: " choice
    
    case $choice in
        0)
            install_service
            read -p "按回车键继续..."
            ;;
        1)
            start_service
            read -p "按回车键继续..."
            ;;
        2)
            stop_service
            read -p "按回车键继续..."
            ;;
        3)
            restart_service
            read -p "按回车键继续..."
            ;;
        4)
            update_panel
            read -p "按回车键继续..."
            ;;
        5)
            force_update
            read -p "按回车键继续..."
            ;;
        6)
            update_script
            ;;
        7)
            view_status
            read -p "按回车键继续..."
            ;;
        8)
            view_logs
            ;;
        9)
            change_port
            read -p "按回车键继续..."
            ;;
        10)
            uninstall
            read -p "按回车键继续..."
            ;;
        11)
            echo -e "${GREEN}退出脚本${NC}"
            exit 0
            ;;
        12)
            select_mirror
            read -p "按回车键继续..."
            ;;
        13)
            select_update_channel
            read -p "按回车键继续..."
            ;;
        *)
            echo -e "${RED}无效选择${NC}"
            read -p "按回车键继续..."
            ;;
    esac
done
