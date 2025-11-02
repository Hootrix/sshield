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
sshield ssh watch                        # 实时监听 SSH 登录并发送通知（推荐 systemd service）
sshield ssh sweep --since 5m             # 处理最近 5 分钟登录事件（默认仅输出）
sshield ssh sweep --since 5m --notify    # 同步发送通知
# 可选参数：--source auto|journal|file，--timezone Asia/Shanghai|Local 等
# 可选参数：--journal-unit sshd.service --log-path /var/log/auth.log 等
```

默认保存位置：
- 配置文件路径：`~/.config/sshield/notify.json`

## 使用示例与调试

```bash
# 开启调试输出（可选）
export SSHIELD_DEBUG=1

# 配置登录密钥（默认生成 ED25519）
sshield ssh key --type ed25519

# 修改 SSH 端口（带确认提示）
sshield ssh port 2201

# 跳过确认直接修改端口
sshield ssh port 2201 --yes

# 禁用密码登录
sshield ssh password-login --disable
```

### 环境变量配置

需要避免在命令历史中暴露敏感参数时，可以预先设置以下环境变量为 `notify email` 提供默认值：

- `SSHIELD_NOTIFY_EMAIL_TO`：收件人邮箱地址
- `SSHIELD_NOTIFY_EMAIL_FROM`：发件人邮箱地址
- `SSHIELD_NOTIFY_EMAIL_SERVER`：SMTP 服务器主机名
- `SSHIELD_NOTIFY_EMAIL_PORT`：SMTP 端口号
- `SSHIELD_NOTIFY_EMAIL_USER`：SMTP 用户名
- `SSHIELD_NOTIFY_EMAIL_PASSWORD`：SMTP 密码

示例：

```bash
export SSHIELD_NOTIFY_EMAIL_TO=ops@example.com
export SSHIELD_NOTIFY_EMAIL_FROM=ssh@example.com
export SSHIELD_NOTIFY_EMAIL_SERVER=smtp.example.com
export SSHIELD_NOTIFY_EMAIL_USER=smtp-user
export SSHIELD_NOTIFY_EMAIL_PASSWORD='super-secret'

sshield notify email
```

> 提示：`notify email` 会对 SMTP 连接设置超时，并在 465 端口自动启用 TLS，避免因网络阻塞造成命令卡住。

## 部署示例

默认未配置通知渠道时，`watch`/`sweep` 仍会将监控结果输出到标准输出，可配合 systemd 日志留档。

### systemd service

```ini
[Unit]
Description=SSHield login watcher
After=network.target

[Service]
ExecStart=/usr/local/bin/sshield ssh watch
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
* * * * * /usr/local/bin/sshield ssh sweep --since 90s >> /var/log/sshield.log 2>&1
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
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w -extldflags "-static -fpic"' -o bin/sshield cmd/sshield/main.go


CGO_ENABLED=0 GOOS=linux GOARCH=386 \
go build -ldflags="-s -w" -o bin/sshield cmd/sshield/main.go


go build -o bin/sshield cmd/sshield/main.go
```
