## 任务

- [x] 在 `internal/core/notify/command.go` 中为含默认值的参数帮助信息补充默认值说明。
- [x] 运行 `gofmt`。
- [x] 视情况提交包含变更的 git commit。
- [x] 在 `internal/core/notify/watcher.go` 中为 `journalRecord` 添加作用说明注释。
- [x] 统一通知配置文件路径逻辑，改为固定使用用户配置目录。
- [x] 更新 README 中的配置路径说明，反映统一后的目录。
- [x] 将 `watch`/`sweep` 命令挂载到 `ssh` 子命令并同步文档示例。
- [x] 优化 Linux 平台 SSH 服务重启逻辑，兼容 `ssh` 服务名称并提供准确提示。
- [x] 修正 `ssh password-login` 状态检测，确保与实际配置一致。
- [x] 修复 Ubuntu 24 下 SSH 端口修改未生效的问题，兼容 Include 配置。
- [x] 增加端口调试日志并完善 drop-in 写入逻辑。
- [x] 调整端口配置策略：使用 drop-in 时仅保留单一 Port 定义。
- [x] 扩充端口调试输出并强制 drop-in 端口覆盖，确保最终生效。
- [x] 改为注释已有 Port 配置（保留原行），避免直接删除。
