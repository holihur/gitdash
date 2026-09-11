#!/bin/sh
# gitdash 包管理器安装后脚本：reload systemd，并准备 /etc/gitdash/gitdash.env。
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload 2>/dev/null || true
    echo "gitdash installed. To start it now:"
    echo "  systemctl enable --now gitdash"
fi

# 配置目录 + 示例环境文件（systemd 通过 EnvironmentFile 读取；含密钥，权限 0600）
mkdir -p /etc/gitdash 2>/dev/null || true
if [ -d /etc/gitdash ] && [ ! -f /etc/gitdash/gitdash.env ]; then
    cat > /etc/gitdash/gitdash.env <<'EOF'
# gitdash 环境变量（systemd 通过 EnvironmentFile 读取；修改后 systemctl restart gitdash）
#
# 管理面板（/admin）：首次启动前设置管理员密码以启用（务必修改为强密码）
#GITDASH_ADMIN_PASSWORD=change-me
#GITDASH_ADMIN_USER=admin
#
# 邮件通知 / 邮箱验证（SMTP，省略则不发送邮件）
#GITDASH_SMTP_HOST=smtp.example.com
#GITDASH_SMTP_PORT=587
#GITDASH_SMTP_USER=you@example.com
#GITDASH_SMTP_PASS=secret
#GITDASH_SMTP_FROM=gitdash <noreply@example.com>
#
# 自动更新（默认关闭）
#GITDASH_AUTO_UPDATE=1
EOF
    chmod 0600 /etc/gitdash/gitdash.env 2>/dev/null || true
fi

echo "Data directory: /var/lib/gitdash (override with GITDASH_DATA)"
echo "Config file:    /etc/gitdash/gitdash.env"
echo "Web UI: http://localhost:8080 - SSH: :2222"
