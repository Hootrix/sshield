# SSHield

SSHield 是 Linux 服务器安全配置工具

## 特性

- 🔐 SSH 安全配置
  - 密钥登录配置
  - 密码安全策略
  - 自定义端口
- 📧 登录通知与监控
  - 基于 journalctl 的实时监听（systemd）
  - Webhook 与 SMTP 邮件通知
  - 支持 cron/systemd timer 的一次性扫尾模式

## 安装

```bash
go install github.com/Hootrix/sshield/cmd/sshield@latest
```

## 使用

```bash
# 查看帮助
sshield --help

# SSH 配置
sshield ssh key --type ed25519           # 配置密钥登录
sshield ssh password-login --disable     # 禁用密码登录
sshield ssh change-password -u user -r   # 为用户生成随机强密码
sshield ssh port -p 2222                 # 修改 SSH 端口

# 登录通知配置
sshield notify webhook --url "https://example.com/webhook"
sshield notify email --to ops@example.com --from ssh@example.com --server smtp.example.com --user smtp-user --password secret
sshield notify test                      # 发送测试通知
sshield notify status                    # 查看当前通知配置

# 登录事件监听
sshield notify watch                     # 实时监听 SSH 登录并发送通知（推荐 systemd service）
sshield notify sweep --since 5m          # 处理最近 5 分钟登录事件（适合 cron/容器）
# 可选参数：--source auto|journal|file，--journal-unit sshd.service --log-path /var/log/auth.log 等
```

默认保存位置：
- root 用户：`/etc/sshield/notify.json`
- 普通用户：`~/.config/sshield/notify.json`

## 部署示例

默认未配置通知渠道时，`watch`/`sweep` 仍会将监控结果输出到标准输出，可配合 systemd 日志留档。

### systemd service

```ini
[Unit]
Description=SSHield login watcher
After=network.target

[Service]
ExecStart=/usr/local/bin/sshield notify watch
Restart=always
User=root

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now sshield-notify.service
```

### cron / 定时任务

```bash
* * * * * /usr/local/bin/sshield notify sweep --since 90s >> /var/log/sshield.log 2>&1
```

## 贡献

欢迎提交 Pull Request 和 Issue。

## 许可证

MIT License

## 开发

1. 克隆仓库
```bash
git clone https://github.com/Hootrix/sshield.git
```

2. 安装依赖
```bash
go mod tidy
```

3. 构建
```bash
go build -o bin/sshield cmd/sshield/main.go
```
