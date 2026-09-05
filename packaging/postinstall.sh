#!/bin/sh
# gitdash 包管理器安装后脚本
# systemd 可用时尝试 reload 并提示启用服务
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload 2>/dev/null || true
    echo "gitdash installed. To start it now:"
    echo "  systemctl enable --now gitdash"
fi
echo "Data directory: /var/lib/gitdash (override with GITDASH_DATA)"
echo "Web UI: http://localhost:8080 - SSH: :2222"
