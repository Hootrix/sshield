package notify

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	sourceAuto    = "auto"
	sourceJournal = "journal"
	sourceFile    = "file"
)

var (
	defaultJournalUnits = []string{"sshd.service", "ssh.service"}
	defaultLogPaths     = []string{"/var/log/auth.log", "/var/log/secure"}
)

type journalRecord struct {
	Cursor     string `json:"__CURSOR"`
	Message    string `json:"MESSAGE"`
	Hostname   string `json:"_HOSTNAME"`
	RealtimeTS string `json:"__REALTIME_TIMESTAMP"`
}

// WatchOptions 控制 watch 模式行为
type WatchOptions struct {
	CursorPath   string
	Source       string
	JournalUnits []string
	LogPaths     []string
	PollTimeout  time.Duration
}

// SweepOptions 控制 sweep 模式行为
type SweepOptions struct {
	CursorPath   string
	Source       string
	JournalUnits []string
	LogPaths     []string
	Since        time.Duration
}

type sourceSelection struct {
	Source      string
	Units       []string
	LogPath     string
	Description string
}

// RunWatch 持续监听 SSH 登录事件
func RunWatch(ctx context.Context, opts WatchOptions) error {
	if opts.PollTimeout <= 0 {
		opts.PollTimeout = 5 * time.Second
	}

	store, err := NewCursorStore(opts.CursorPath)
	if err != nil {
		return err
	}

	state, err := store.Load()
	if err != nil {
		return err
	}

	selection, err := determineSource(opts.Source, opts.JournalUnits, opts.LogPaths)
	if err != nil {
		return err
	}

	fmt.Printf(">>> 监听模式：%s\n", selection.Description)

	switch selection.Source {
	case sourceJournal:
		return runJournal(ctx, store, state, selection.Units, opts.PollTimeout, true, 0)
	case sourceFile:
		return followLogFile(ctx, store, state, selection.LogPath, opts.PollTimeout)
	default:
		return fmt.Errorf("未知监听源: %s", selection.Source)
	}
}

// RunSweep 处理近期 SSH 登录事件后退出
func RunSweep(ctx context.Context, opts SweepOptions) error {
	store, err := NewCursorStore(opts.CursorPath)
	if err != nil {
		return err
	}

	state, err := store.Load()
	if err != nil {
		return err
	}

	selection, err := determineSource(opts.Source, opts.JournalUnits, opts.LogPaths)
	if err != nil {
		return err
	}

	fmt.Printf(">>> 扫描模式：%s\n", selection.Description)

	switch selection.Source {
	case sourceJournal:
		return runJournal(ctx, store, state, selection.Units, 0, false, opts.Since)
	case sourceFile:
		return sweepLogFile(ctx, store, state, selection.LogPath)
	default:
		return fmt.Errorf("未知监听源: %s", selection.Source)
	}
}

func determineSource(source string, units, paths []string) (*sourceSelection, error) {
	s := &sourceSelection{}

	if len(units) == 0 {
		units = make([]string, len(defaultJournalUnits))
		copy(units, defaultJournalUnits)
	}
	if len(paths) == 0 {
		paths = make([]string, len(defaultLogPaths))
		copy(paths, defaultLogPaths)
	}

	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = sourceAuto
	}

	journalOK, journalHasEntries := probeJournal(units)
	logPath, logExists := firstExisting(paths)

	switch source {
	case sourceJournal:
		if !journalOK {
			return nil, fmt.Errorf("journalctl 不可用或无法访问")
		}
		s.Source = sourceJournal
		s.Units = units
		s.Description = fmt.Sprintf("journald（units=%v）", units)
		return s, nil
	case sourceFile:
		if !logExists {
			return nil, fmt.Errorf("未找到有效的日志文件：%v", paths)
		}
		s.Source = sourceFile
		s.LogPath = logPath
		s.Description = fmt.Sprintf("文件日志：%s", logPath)
		return s, nil
	case sourceAuto:
		if journalOK && (journalHasEntries || !logExists) {
			s.Source = sourceJournal
			s.Units = units
			s.Description = fmt.Sprintf("journald（units=%v）", units)
			if !journalHasEntries {
				s.Description += "，尚无历史记录"
			}
			return s, nil
		}
		if logExists {
			s.Source = sourceFile
			s.LogPath = logPath
			s.Description = fmt.Sprintf("文件日志：%s", logPath)
			return s, nil
		}
		if journalOK {
			s.Source = sourceJournal
			s.Units = units
			s.Description = fmt.Sprintf("journald（units=%v）", units)
			return s, nil
		}
		return nil, fmt.Errorf("未检测到可用的日志源（journalctl 不可用，且未找到 %v）", paths)
	default:
		return nil, fmt.Errorf("不支持的 source：%s（可选 auto|journal|file）", source)
	}
}

func probeJournal(units []string) (available bool, hasEntries bool) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return false, false
	}

	args := []string{"--no-pager", "-n", "50"}
	for _, unit := range units {
		args = append(args, "-u", unit)
	}

	cmd := exec.Command("journalctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, false
	}

	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return true, false
	}
	if bytes.Contains(trimmed, []byte("-- No entries --")) {
		return true, false
	}
	// journalctl 输出涵盖多行头信息，包含 "sshd" 视为有记录
	if bytes.Contains(bytes.ToLower(trimmed), []byte("sshd")) || bytes.Contains(bytes.ToLower(trimmed), []byte("ssh")) {
		return true, true
	}
	return true, false
}

func firstExisting(paths []string) (string, bool) {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p, true
		}
	}
	return "", false
}

func runJournal(ctx context.Context, store *CursorStore, state *SourceState, units []string, poll time.Duration, follow bool, since time.Duration) error {
	if state == nil {
		state = &SourceState{}
	}

	args := []string{"--no-pager", "-o", "json"}
	if follow {
		args = append(args, "--follow")
	}
	for _, unit := range units {
		args = append(args, "-u", unit)
	}

	if state.JournalCursor != "" {
		args = append(args, "--after-cursor", state.JournalCursor)
	} else if !follow && since > 0 {
		sinceTime := time.Now().Add(-since).Format("2006-01-02 15:04:05")
		args = append(args, "--since", sinceTime)
	} else if follow {
		args = append(args, "--since", "now")
	}

	if _, err := exec.LookPath("journalctl"); err != nil {
		return fmt.Errorf("未找到 journalctl，请确认当前系统使用 systemd 并已安装相应工具")
	}

	cmd := exec.CommandContext(ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("读取 journalctl 输出失败: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 journalctl 失败: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			_ = cmd.Wait()
			return nil
		default:
		}

		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var record journalRecord
		if err := json.Unmarshal(line, &record); err != nil {
			log.Printf("解析 journald 输出失败: %v", err)
			continue
		}

		ts := parseRealtime(record.RealtimeTS)
		event, ok := parseJournalMessage(record.Message, record.Hostname, ts)
		if !ok {
			continue
		}

		if err := dispatchEvent(event); err != nil {
			log.Printf("发送通知失败: %v", err)
		}

		printEventSummary(*event)

		state.JournalCursor = record.Cursor
		if err := store.Save(state); err != nil {
			log.Printf("写入状态失败: %v", err)
		}
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("读取 journald 输出失败: %w", err)
	}

	if follow {
		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("journalctl 退出: %w", err)
		}
		return nil
	}

	return cmd.Wait()
}

func followLogFile(ctx context.Context, store *CursorStore, state *SourceState, path string, poll time.Duration) error {
	if poll <= 0 {
		poll = time.Second
	}

	offset := state.FileOffsets[path]
	if offset < 0 {
		offset = 0
	}

	process := func(event *LoginEvent, newOffset int64) {
		if err := dispatchEvent(event); err != nil {
			log.Printf("发送通知失败: %v", err)
		}
		printEventSummary(*event)
		offset = newOffset
		state.FileOffsets[path] = offset
		if err := store.Save(state); err != nil {
			log.Printf("写入状态失败: %v", err)
		}
	}

	for {
		_, err := readLogFile(ctx, path, offset, process, true, poll)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			log.Printf("读取 %s 失败: %v", path, err)
			time.Sleep(poll)
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}
}

func sweepLogFile(ctx context.Context, store *CursorStore, state *SourceState, path string) error {
	offset := state.FileOffsets[path]
	var latest int64 = offset
	process := func(event *LoginEvent, newOffset int64) {
		if err := dispatchEvent(event); err != nil {
			log.Printf("发送通知失败: %v", err)
		}
		printEventSummary(*event)
		latest = newOffset
	}
	_, err := readLogFile(ctx, path, offset, process, false, 0)
	if err != nil {
		return err
	}
	state.FileOffsets[path] = latest
	return store.Save(state)
}

func readLogFile(ctx context.Context, path string, startOffset int64, handle func(*LoginEvent, int64), follow bool, poll time.Duration) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return startOffset, fmt.Errorf("打开日志文件失败: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return startOffset, fmt.Errorf("读取文件信息失败: %w", err)
	}

	offset := startOffset
	if offset == 0 && follow {
		offset = info.Size()
	}
	if offset > info.Size() {
		offset = info.Size()
	}

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset, fmt.Errorf("定位文件偏移失败: %w", err)
	}

	reader := bufio.NewReader(file)
	for {
		select {
		case <-ctx.Done():
			return offset, context.Canceled
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !follow {
					return offset, nil
				}
				time.Sleep(poll)
				if err := reopenIfRotated(&file, path, &reader, &offset); err != nil {
					log.Printf("处理日志轮转失败: %v", err)
				}
				continue
			}
			return offset, fmt.Errorf("读取日志失败: %w", err)
		}

		offset += int64(len(line))
		event, ok := parseAuthLogLine(strings.TrimRight(line, "\r\n"))
		if !ok {
			continue
		}
		handle(event, offset)
	}
}

func reopenIfRotated(file **os.File, path string, reader **bufio.Reader, offset *int64) error {
	currentInfo, err := (*file).Stat()
	if err != nil {
		return err
	}

	newInfo, err := os.Stat(path)
	if err != nil {
		return err
	}

	if os.SameFile(currentInfo, newInfo) && newInfo.Size() >= *offset {
		return nil
	}

	(*file).Close()

	newFile, err := os.Open(path)
	if err != nil {
		return err
	}

	*file = newFile
	*reader = bufio.NewReader(newFile)
	*offset = 0
	return nil
}

func parseRealtime(ts string) time.Time {
	if ts == "" {
		return time.Now()
	}
	val, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Now()
	}
	return time.UnixMicro(val)
}

func dispatchEvent(event *LoginEvent) error {
	cfg, err := loadConfig()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) || errors.Is(err, ErrNotEnabled) {
			return nil
		}
		return fmt.Errorf("读取通知配置失败: %w", err)
	}
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	notifier, err := buildNotifier(*cfg)
	if err != nil {
		return err
	}

	return notifier.Send(*event)
}

func buildNotifier(cfg Config) (Notifier, error) {
	switch strings.ToLower(cfg.Type) {
	case "webhook":
		return NewWebhookNotifier(cfg.WebhookURL), nil
	case "email":
		return NewEmailNotifier(cfg), nil
	default:
		return nil, fmt.Errorf("未知通知类型: %s", cfg.Type)
	}
}

func printEventSummary(event LoginEvent) {
	method := event.Method
	if method == "" {
		method = "-"
	}

	port := "-"
	if event.Port > 0 {
		port = fmt.Sprintf("%d", event.Port)
	}

	fmt.Fprintf(os.Stdout, "[%s] %s 用户=%s IP=%s 端口=%s 方式=%s 主机=%s\n",
		event.Timestamp.Format(time.RFC3339),
		event.Type,
		event.User,
		event.IP,
		port,
		method,
		event.Hostname,
	)
}
