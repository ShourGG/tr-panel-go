#!/bin/bash

set -e

echo "========================================="
echo "  TR Panel 一键安装脚本"
echo "========================================="
echo ""

# 检查是否为root用户
if [ "$EUID" -ne 0 ]; then 
    echo "错误: 请使用 root 用户或 sudo 运行此脚本"
    exit 1
fi

# 定义变量
INSTALL_DIR="/opt/tr-panel"
SERVICE_NAME="tr-panel"
VERSION="v1.0.0"
DOWNLOAD_URL="https://github.com/ShourGG/tr-panel-go/releases/download/${VERSION}/tr-panel-linux-amd64"

echo "[1/5] 创建安装目录..."
mkdir -p $INSTALL_DIR
cd $INSTALL_DIR

echo "[2/5] 下载 TR Panel ${VERSION}..."
if command -v wget &> /dev/null; then
    wget -O tr-panel $DOWNLOAD_URL
elif command -v curl &> /dev/null; then
    curl -L -o tr-panel $DOWNLOAD_URL
else
    echo "错误: 需要安装 wget 或 curl"
    exit 1
fi

echo "[3/5] 设置执行权限..."
chmod +x tr-panel

echo "[4/5] 创建 systemd 服务..."
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

[Install]
WantedBy=multi-user.target
EOF

echo "[5/5] 启动服务..."
systemctl daemon-reload
systemctl enable $SERVICE_NAME
systemctl start $SERVICE_NAME

echo ""
echo "========================================="
echo "  安装完成！"
echo "========================================="
echo ""
echo "面板地址: http://$(hostname -I | awk '{print $1}'):8800"
echo ""
echo "常用命令:"
echo "  启动: systemctl start $SERVICE_NAME"
echo "  停止: systemctl stop $SERVICE_NAME"
echo "  重启: systemctl restart $SERVICE_NAME"
echo "  状态: systemctl status $SERVICE_NAME"
echo "  日志: journalctl -u $SERVICE_NAME -f"
echo ""
