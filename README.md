# SSHield

SSHield 是 Linux 服务器安全配置工具

## 特性

- 🔐 SSH 安全配置
  - 密钥登录配置
  - 密码安全策略
  - 自定义端口
<!-- - 🛡️ 防火墙管理
  - UFW
  - Firewalld -->
- 📧 登录通知[TODO]
  - Webhook 支持
  - 邮件通知

## 安装

```bash
go install github.com/Hootrix/sshield/cmd/sshield@latest
```

## 使用

```bash
# 查看帮助
sshield --help

# SSH 配置
sshield ssh key-only    # 配置仅密钥登录
sshield ssh password    # 配置密码策略
sshield ssh port -p 2222 # 修改 SSH 端口

# 防火墙配置
sshield firewall setup
sshield firewall status
sshield firewall rule

# 配置登录通知
sshield notify webhook -u "YOUR_WEBHOOK_URL"
sshield notify email -e "your@email.com"
sshield notify test
```

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

## 贡献

欢迎提交 Pull Request 和 Issue。

## 许可证

MIT License
