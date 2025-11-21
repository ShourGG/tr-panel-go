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
SCRIPT_VERSION="1.0.0"

# 定义变量
INSTALL_DIR="/opt/tr-panel"
SERVICE_NAME="tr-panel"
VERSION="v1.0.0"
DOWNLOAD_URL="https://github.com/ShourGG/tr-panel-go/releases/download/${VERSION}/tr-panel-linux-amd64"
PORT=8800

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

# 版本比较函数
version_gt() {
    test "$(printf '%s\n' "$@" | sort -V | head -n 1)" != "$1"
}

# 检查脚本更新
check_script_update() {
    # 获取远程版本号（超时1秒）
    REMOTE_VERSION=$(timeout 1 curl -s --connect-timeout 1 --max-time 1 https://raw.githubusercontent.com/ShourGG/tr-panel-go/main/tr.sh 2>/dev/null | grep "^SCRIPT_VERSION=" | head -1 | cut -d'"' -f2)
    
    if [ -z "$REMOTE_VERSION" ]; then
        return
    fi
    
    # 比较版本
    if version_gt "$REMOTE_VERSION" "$SCRIPT_VERSION"; then
        echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e "${YELLOW}  发现新版本！${NC}"
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
    
    # 检查服务状态
    if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
        STATUS="${GREEN}● 运行中${NC}"
    else
        STATUS="${RED}○ 已停止${NC}"
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
    echo "[9]: 卸载面板 (Uninstall)"
    echo "[10]: 退出脚本 (Exit)"
    echo "————————————————————————————————————————"
    echo ""
}

# 下载并启动
install_service() {
    check_root
    echo -e "${GREEN}[1/5] 创建安装目录...${NC}"
    mkdir -p $INSTALL_DIR
    cd $INSTALL_DIR

    echo -e "${GREEN}[2/5] 下载 TR Panel ${VERSION}...${NC}"
    if command -v wget &> /dev/null; then
        wget -O tr-panel $DOWNLOAD_URL
    elif command -v curl &> /dev/null; then
        curl -L -o tr-panel $DOWNLOAD_URL
    else
        echo -e "${RED}错误: 需要安装 wget 或 curl${NC}"
        exit 1
    fi

    echo -e "${GREEN}[3/5] 设置执行权限...${NC}"
    chmod +x tr-panel

    echo -e "${GREEN}[4/5] 创建 systemd 服务...${NC}"
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

[Install]
WantedBy=multi-user.target
EOF

    echo -e "${GREEN}[5/5] 启动服务...${NC}"
    systemctl daemon-reload
    systemctl enable $SERVICE_NAME
    systemctl start $SERVICE_NAME

    echo ""
    echo -e "${GREEN}=========================================${NC}"
    echo -e "${GREEN}  安装完成！${NC}"
    echo -e "${GREEN}=========================================${NC}"
    echo ""
    echo -e "面板地址: ${GREEN}http://$(hostname -I | awk '{print $1}'):${PORT}${NC}"
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
    echo -e "${GREEN}开始更新面板...${NC}"
    systemctl stop $SERVICE_NAME
    cd $INSTALL_DIR
    rm -f tr-panel
    
    if command -v wget &> /dev/null; then
        wget -O tr-panel $DOWNLOAD_URL
    elif command -v curl &> /dev/null; then
        curl -L -o tr-panel $DOWNLOAD_URL
    fi
    
    chmod +x tr-panel
    systemctl start $SERVICE_NAME
    echo -e "${GREEN}更新完成！${NC}"
}

# 强制更新
force_update() {
    check_root
    echo -e "${YELLOW}强制更新面板...${NC}"
    systemctl stop $SERVICE_NAME
    rm -rf $INSTALL_DIR
    install_service
}

# 更新脚本
update_script() {
    echo -e "${GREEN}更新脚本...${NC}"
    cd ~ && rm -f tr.sh
    wget -O tr.sh https://raw.githubusercontent.com/ShourGG/tr-panel-go/main/tr.sh
    chmod +x tr.sh
    echo -e "${GREEN}脚本已更新，请重新运行: ./tr.sh${NC}"
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

# 主循环
while true; do
    show_menu
    read -p "请输入选择 (Please enter your selection) [0-10]: " choice
    
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
            uninstall
            read -p "按回车键继续..."
            ;;
        10)
            echo -e "${GREEN}退出脚本${NC}"
            exit 0
            ;;
        *)
            echo -e "${RED}无效选择${NC}"
            read -p "按回车键继续..."
            ;;
    esac
done
