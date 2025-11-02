package ssh

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// KeyType 定义SSH密钥类型
type KeyType string

const (
	// KeyTypeEd25519 使用Ed25519算法（推荐）
	KeyTypeEd25519 KeyType = "ed25519"
	// KeyTypeRSA 使用RSA算法（用于兼容旧系统）
	KeyTypeRSA KeyType = "rsa"

	sshConfigPath  = "/etc/ssh/sshd_config"
	defaultKeyPath = "~/.ssh/id_rsa"
)

// KeyTypeConfig 定义密钥生成的配置
type KeyTypeConfig struct {
	Type KeyType
	Bits int // RSA密钥长度
}

// DefaultKeyConfig 返回推荐的密钥配置
func DefaultKeyConfig() KeyTypeConfig {
	return KeyTypeConfig{
		Type: KeyTypeEd25519,
	}
}

// SSHAuthConfig SSH认证配置
type SSHAuthConfig struct {
	KeyType         KeyTypeConfig // 密钥类型配置
	DisablePassword bool          // 是否禁用密码登录
}

// DefaultAuthConfig 返回默认的SSH认证配置
func DefaultAuthConfig() SSHAuthConfig {
	return SSHAuthConfig{
		KeyType:         DefaultKeyConfig(),
		DisablePassword: false, // 默认不禁用密码登录
	}
}

// 检查是否存在SSH密钥
func checkSSHKey(keyPath string) bool {
	expandedPath := expandPath(keyPath)
	_, err := os.Stat(expandedPath)
	return err == nil
}

// 生成新的SSH密钥对
func generateSSHKey(keyPath string, email string, config KeyTypeConfig) error {
	// 检查密钥是否已存在
	if _, err := os.Stat(keyPath); err == nil {
		return fmt.Errorf("密钥已存在：%s", keyPath)
	}

	// 生成密钥
	var cmd *exec.Cmd
	switch config.Type {
	case KeyTypeEd25519:
		cmd = exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-C", email, "-N", "")
	case KeyTypeRSA:
		cmd = exec.Command("ssh-keygen", "-t", "rsa", "-b", strconv.Itoa(config.Bits), "-f", keyPath, "-C", email, "-N", "")
	default:
		return fmt.Errorf("不支持的密钥类型：%s", config.Type)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("生成密钥失败: %v\n%s", err, output)
	}

	return nil
}

// 将公钥添加到authorized_keys文件
func addToAuthorizedKeys(keyPath string) error {
	expandedPath := expandPath(keyPath)
	pubKeyPath := expandedPath

	// 读取公钥内容
	pubKey, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("读取公钥文件失败: %v", err)
	}

	// 确保authorized_keys文件存在
	authKeysPath := filepath.Join(filepath.Dir(expandedPath), "authorized_keys")
	if err := ensureFile(authKeysPath, 0600); err != nil {
		return fmt.Errorf("创建authorized_keys文件失败: %v", err)
	}

	// 检查公钥是否已经存在
	currentContent, err := os.ReadFile(authKeysPath)
	if err != nil {
		return fmt.Errorf("读取authorized_keys文件失败: %v", err)
	}

	if strings.Contains(string(currentContent), string(pubKey)) {
		return nil // 公钥已存在
	}

	// 追加公钥到文件
	f, err := os.OpenFile(authKeysPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("打开authorized_keys文件失败: %v", err)
	}
	defer f.Close()

	if _, err := f.Write(pubKey); err != nil {
		return fmt.Errorf("写入公钥失败: %v", err)
	}

	return nil
}

// 确保文件存在
func ensureFile(path string, perm os.FileMode) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, perm)
		if err != nil {
			return err
		}
		f.Close()
	}
	return nil
}

// 展开路径中的~
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return path
}

// 设置仅密钥认证前的准备工作
func prepareKeyAuth(email string, config KeyTypeConfig) error {
	// 获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录失败: %v", err)
	}

	// 创建 .ssh 目录
	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("创建 .ssh 目录失败: %v", err)
	}

	// 生成密钥对
	keyPath := filepath.Join(sshDir, fmt.Sprintf("id_%s", config.Type))
	if err := generateSSHKey(keyPath, email, config); err != nil {
		return fmt.Errorf("生成密钥失败: %v", err)
	}

	// 添加到 authorized_keys
	if err := addToAuthorizedKeys(keyPath + ".pub"); err != nil {
		return fmt.Errorf("添加公钥到 authorized_keys 失败: %v", err)
	}

	return nil
}

// getRestartCommands 返回可尝试的重启 SSH 服务命令。
func getRestartCommands() []string {
	switch runtime.GOOS {
	case "linux":
		serviceNames := []string{"sshd", "ssh"}
		var commands []string
		if _, err := exec.LookPath("systemctl"); err == nil {
			for _, svc := range serviceNames {
				commands = append(commands, fmt.Sprintf("sudo systemctl restart %s", svc))
			}
		}
		if _, err := exec.LookPath("service"); err == nil {
			for _, svc := range serviceNames {
				commands = append(commands, fmt.Sprintf("sudo service %s restart", svc))
			}
		}
		if len(commands) == 0 {
			for _, svc := range serviceNames {
				commands = append(commands, fmt.Sprintf("sudo systemctl restart %s", svc))
			}
		}
		return commands
	case "darwin":
		return []string{"sudo launchctl kickstart -k system/com.openssh.sshd"}
	default:
		return nil
	}
}

type restartAttempt struct {
	command []string
	hint    string
}

func linuxRestartAttempts() ([]restartAttempt, []string) {
	var attempts []restartAttempt
	serviceNames := []string{"sshd", "ssh"}

	if _, err := exec.LookPath("systemctl"); err == nil {
		for _, svc := range serviceNames {
			attempts = append(attempts, restartAttempt{
				command: []string{"systemctl", "restart", svc},
				hint:    fmt.Sprintf("sudo systemctl restart %s", svc),
			})
		}
	}

	if _, err := exec.LookPath("service"); err == nil {
		for _, svc := range serviceNames {
			attempts = append(attempts, restartAttempt{
				command: []string{"service", svc, "restart"},
				hint:    fmt.Sprintf("sudo service %s restart", svc),
			})
		}
	}

	hints := make([]string, len(attempts))
	for i, attempt := range attempts {
		hints[i] = attempt.hint
	}
	if len(hints) == 0 {
		hints = getRestartCommands()
	}
	return attempts, hints
}

// restartSSHService 重启 SSH 服务
func restartSSHService() error {
	// 检查操作系统类型并执行相应的重启命令
	switch runtime.GOOS {
	case "linux":
		attempts, hints := linuxRestartAttempts()
		if len(attempts) == 0 {
			return fmt.Errorf("未找到支持的服务管理器。请手动执行以下命令重启服务：\n  %s", strings.Join(hints, "\n  "))
		}

		var lastErr error
		for _, attempt := range attempts {
			cmd := exec.Command(attempt.command[0], attempt.command[1:]...)
			if err := cmd.Run(); err != nil {
				lastErr = err
				continue
			}
			return nil
		}
		return fmt.Errorf("重启 SSH 服务失败。请手动执行以下命令重启服务：\n  %s\n\n错误信息：%v", strings.Join(hints, "\n  "), lastErr)
	case "darwin":
		cmd := exec.Command("sudo", "launchctl", "kickstart", "-k", "system/com.openssh.sshd")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("重启 SSH 服务失败。请手动执行以下命令重启服务：\n  %s\n\n错误信息：%v", strings.Join(getRestartCommands(), "\n  "), err)
		}
	default:
		return fmt.Errorf("不支持的操作系统: %s。请手动重启 SSH 服务", runtime.GOOS)
	}
	return nil
}

// ConfigureAuth 配置SSH认证方式
func ConfigureAuth(config SSHAuthConfig) error {
	// 备份配置文件
	if err := backupConfig("ConfigureAuth"); err != nil {
		return fmt.Errorf("备份配置文件失败: %v", err)
	}

	// 读取配置文件
	content, err := os.ReadFile(sshConfigPath)
	if err != nil {
		return fmt.Errorf("读取SSH配置文件失败: %v", err)
	}

	// 修改配置
	lines := strings.Split(string(content), "\n")
	var newLines []string
	passwordAuthFound := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "PasswordAuthentication") {
			passwordAuthFound = true
			if config.DisablePassword {
				newLines = append(newLines, "PasswordAuthentication no")
			} else {
				newLines = append(newLines, "PasswordAuthentication yes")
			}
		} else if strings.HasPrefix(trimmedLine, "ChallengeResponseAuthentication") {
			if config.DisablePassword {
				newLines = append(newLines, "ChallengeResponseAuthentication no")
			} else {
				newLines = append(newLines, "ChallengeResponseAuthentication yes")
			}
		} else {
			newLines = append(newLines, line)
		}
	}

	// 如果没有找到 PasswordAuthentication，添加它
	if !passwordAuthFound {
		if config.DisablePassword {
			newLines = append(newLines, "PasswordAuthentication no")
		} else {
			newLines = append(newLines, "PasswordAuthentication yes")
		}
	}

	// 写入新配置
	if err := os.WriteFile(sshConfigPath, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("写入SSH配置文件失败: %v", err)
	}

	// 重启SSH服务
	if err := restartSSHService(); err != nil {
		fmt.Printf("警告：%v\n", err)
		fmt.Println("配置已更新，但需要重启 SSH 服务才能生效。")
		return nil
	}

	return nil
}

func rewritePortContent(content string, port int) (string, bool) {
	lines := strings.Split(content, "\n")
	updated := false

	for i, originalLine := range lines {
		trimmed := strings.TrimSpace(originalLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		withoutComment := originalLine
		comment := ""
		if idx := strings.Index(originalLine, "#"); idx != -1 {
			withoutComment = strings.TrimRight(originalLine[:idx], " \t")
			comment = originalLine[idx:]
		}

		fields := strings.Fields(withoutComment)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "Port") {
			indentLen := len(originalLine) - len(strings.TrimLeft(originalLine, " \t"))
			indent := originalLine[:indentLen]

			newLine := fmt.Sprintf("%sPort %d", indent, port)
			if comment != "" {
				if !strings.HasPrefix(comment, " ") {
					newLine += " "
				}
				newLine += strings.TrimLeft(comment, " ")
			}
			lines[i] = newLine
			updated = true
		}
	}

	return strings.Join(lines, "\n"), updated
}

func rewritePortFile(path string, port int) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	newContent, updated := rewritePortContent(string(content), port)
	if !updated {
		return false, nil
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return false, err
	}
	return true, nil
}

func parseIncludePatterns(content string) []string {
	lines := strings.Split(content, "\n")
	baseDir := filepath.Dir(sshConfigPath)
	seen := make(map[string]struct{})
	var patterns []string

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if idx := strings.Index(line, "#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}

		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.EqualFold(fields[0], "Include") {
			continue
		}

		for _, pattern := range fields[1:] {
			pattern = strings.Trim(pattern, "\"")
			if pattern == "" {
				continue
			}

			expanded := expandPath(pattern)
			if !filepath.IsAbs(expanded) {
				expanded = filepath.Join(baseDir, expanded)
			}

			if _, ok := seen[expanded]; ok {
				continue
			}
			seen[expanded] = struct{}{}
			patterns = append(patterns, expanded)
		}
	}

	return patterns
}

func changePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("端口号必须在1-65535之间")
	}

	// 先备份配置文件
	if err := backupConfig("changePort"); err != nil {
		return fmt.Errorf("备份配置文件失败: %v", err)
	}

	// 读取配置文件
	content, err := os.ReadFile(sshConfigPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	originalContent := string(content)

	newContent, updated := rewritePortContent(originalContent, port)
	if updated {
		if err := os.WriteFile(sshConfigPath, []byte(newContent), 0644); err != nil {
			return fmt.Errorf("写入配置文件失败: %v", err)
		}
		content = []byte(newContent)
	}

	includePatterns := parseIncludePatterns(originalContent)
	portConfigured := updated

	for _, pattern := range includePatterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}

			changed, err := rewritePortFile(match, port)
			if err != nil {
				return fmt.Errorf("更新 %s 失败: %w", match, err)
			}
			if changed {
				portConfigured = true
			}
		}
	}

	if !portConfigured {
		finalContent := strings.TrimRight(string(content), "\n")
		if finalContent != "" {
			finalContent += "\n"
		}
		finalContent += fmt.Sprintf("Port %d\n", port)
		if err := os.WriteFile(sshConfigPath, []byte(finalContent), 0644); err != nil {
			return fmt.Errorf("写入配置文件失败: %v", err)
		}
	}

	// 重启服务
	if err := restartSSHService(); err != nil {
		fmt.Printf("警告：%v\n", err)
		fmt.Println("配置已更新，但需要重启 SSH 服务才能生效。")
		return nil
	}

	return nil
}

func backupConfig(opname string) error {
	srcFile, err := os.Open(sshConfigPath)
	if err != nil {
		return fmt.Errorf("打开SSH配置文件失败: %v", err)
	}
	defer srcFile.Close()

	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.sshield_%s.backup_%s", sshConfigPath, opname, timestamp)

	dstFile, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %v", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("复制配置文件失败: %v", err)
	}

	return nil
}

func restoreConfig() error {
	// 查找最新的备份文件
	backupPattern := sshConfigPath + ".backup_*"
	matches, err := filepath.Glob(backupPattern)
	if err != nil {
		return fmt.Errorf("查找备份文件失败: %v", err)
	}

	if len(matches) == 0 {
		return fmt.Errorf("未找到备份文件")
	}

	// 找到最新的备份文件
	var latestBackup string
	var latestTime time.Time

	for _, match := range matches {
		fi, err := os.Stat(match)
		if err != nil {
			continue
		}
		if latestBackup == "" || fi.ModTime().After(latestTime) {
			latestBackup = match
			latestTime = fi.ModTime()
		}
	}

	// 复制最新的备份文件到原配置文件
	srcFile, err := os.Open(latestBackup)
	if err != nil {
		return fmt.Errorf("打开备份文件失败: %v", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(sshConfigPath)
	if err != nil {
		return fmt.Errorf("创建配置文件失败: %v", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("恢复配置文件失败: %v", err)
	}

	return nil
}

// changePassword 修改用户密码
func changePassword(username, newPassword string) error {
	// 使用chpasswd命令修改密码
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", username, newPassword))

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("修改密码失败: %v, 输出: %s", err, string(output))
	}

	return nil
}

// validatePassword 验证密码强度
func validatePassword(password string) error {
	if len(password) < 16 {
		return fmt.Errorf("密码长度必须大于等于16位")
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	var missing []string
	if !hasUpper {
		missing = append(missing, "大写字母")
	}
	if !hasLower {
		missing = append(missing, "小写字母")
	}
	if !hasNumber {
		missing = append(missing, "数字")
	}
	if !hasSpecial {
		missing = append(missing, "特殊字符")
	}

	if len(missing) > 0 {
		return fmt.Errorf("密码必须包含: %s", strings.Join(missing, "、"))
	}

	return nil
}

// generateRandomPassword 生成随机强密码
func generateRandomPassword(length int) (string, error) {
	if length < 16 {
		length = 20 // 默认20位密码
	}

	// 定义字符集
	const (
		upperChars   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lowerChars   = "abcdefghijklmnopqrstuvwxyz"
		numberChars  = "0123456789"
		specialChars = "!@#$%^&*()_+-=[]{}|;:,.<>?"
	)

	// 安全地生成随机索引
	randomIndex := func(max int) (int, error) {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
		if err != nil {
			return 0, err
		}
		return int(n.Int64()), nil
	}

	// 确保至少包含每种字符
	password := make([]byte, length)

	// 确保至少包含一个大写字母
	idx, err := randomIndex(len(upperChars))
	if err != nil {
		return "", fmt.Errorf("生成随机密码失败: %v", err)
	}
	password[0] = upperChars[idx]

	// 确保至少包含一个小写字母
	idx, err = randomIndex(len(lowerChars))
	if err != nil {
		return "", fmt.Errorf("生成随机密码失败: %v", err)
	}
	password[1] = lowerChars[idx]

	// 确保至少包含一个数字
	idx, err = randomIndex(len(numberChars))
	if err != nil {
		return "", fmt.Errorf("生成随机密码失败: %v", err)
	}
	password[2] = numberChars[idx]

	// 确保至少包含一个特殊字符
	idx, err = randomIndex(len(specialChars))
	if err != nil {
		return "", fmt.Errorf("生成随机密码失败: %v", err)
	}
	password[3] = specialChars[idx]

	// 填充剩余字符
	allChars := upperChars + lowerChars + numberChars + specialChars
	for i := 4; i < length; i++ {
		idx, err = randomIndex(len(allChars))
		if err != nil {
			return "", fmt.Errorf("生成随机密码失败: %v", err)
		}
		password[i] = allChars[idx]
	}

	// 安全地打乱密码顺序
	for i := len(password) - 1; i > 0; i-- {
		j, err := randomIndex(i + 1)
		if err != nil {
			return "", fmt.Errorf("生成随机密码失败: %v", err)
		}
		password[i], password[j] = password[j], password[i]
	}

	return string(password), nil
}

// GetPasswordAuthStatus 获取密码登录的状态
func GetPasswordAuthStatus() (bool, error) {
	content, err := os.ReadFile(sshConfigPath)
	if err != nil {
		return false, fmt.Errorf("读取SSH配置文件失败: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	passwordSet := false
	passwordEnabled := true // OpenSSH 默认启用密码登录
	challengeSet := false
	challengeEnabled := false

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if idx := strings.Index(line, "#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.ToLower(fields[0])
		value := strings.ToLower(fields[1])

		switch key {
		case "passwordauthentication":
			passwordSet = true
			passwordEnabled = value == "yes"
		case "challengeresponseauthentication":
			challengeSet = true
			challengeEnabled = value == "yes"
		}
	}

	if passwordSet {
		return passwordEnabled, nil
	}

	if challengeSet {
		return challengeEnabled, nil
	}

	return true, nil
}

// GetPubKeyAuthStatus 获取密钥认证状态
func GetPubKeyAuthStatus() (bool, error) {
	content, err := os.ReadFile(sshConfigPath)
	if err != nil {
		return false, fmt.Errorf("读取SSH配置文件失败: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "PubkeyAuthentication") {
			return strings.Contains(trimmedLine, "yes"), nil
		}
	}

	// 如果没有找到配置，默认是启用的
	return true, nil
}

// GetSSHPort 获取当前SSH端口
func GetSSHPort() (int, error) {
	content, err := os.ReadFile(sshConfigPath)
	if err != nil {
		return 22, fmt.Errorf("读取SSH配置文件失败: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "Port ") {
			portStr := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "Port"))
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return 22, fmt.Errorf("解析端口号失败: %v", err)
			}
			return port, nil
		}
	}

	// 如果没有找到配置，返回默认端口
	return 22, nil
}

// DebugSSHConfig 显示SSH配置信息
func DebugSSHConfig() string {
	content, err := os.ReadFile(sshConfigPath)
	if err != nil {
		return fmt.Sprintf("读取配置文件失败: %v", err)
	}

	var result strings.Builder
	result.WriteString("配置文件内容：\n")

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}

		// 显示原始行（包含注释）
		if strings.HasPrefix(line, "PasswordAuthentication") ||
			strings.HasPrefix(line, "ChallengeResponseAuthentication") {
			result.WriteString(fmt.Sprintf(">>> %s\n", line))

			// 如果有行尾注释，显示解析后的值
			if strings.Contains(line, "#") {
				if idx := strings.Index(line, "#"); idx != -1 {
					effectiveLine := strings.TrimSpace(line[:idx])
					parts := strings.Fields(effectiveLine)
					if len(parts) >= 2 {
						result.WriteString(fmt.Sprintf("   实际生效值：%s\n", parts[1]))
					}
				}
			}
		}
	}

	return result.String()
}
