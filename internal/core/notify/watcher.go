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

const journalHistoryTolerance = time.Minute

// 事件去重：同一 IP+Port+User 在此时间窗口内只通知一次
const eventDedupeWindow = 5 * time.Second

// NotifyOn 控制通知哪些类型的事件
type NotifyOn string

const (
	NotifyOnAll     NotifyOn = "all"
	NotifyOnSuccess NotifyOn = "success"
	NotifyOnFailed  NotifyOn = "failed"
)

// eventDeduper 用于去重短时间内的重复事件（如 VERBOSE 级别下 Failed + Disconnected）
type eventDeduper struct {
	seen map[string]time.Time
}

func newEventDeduper() *eventDeduper {
	return &eventDeduper{seen: make(map[string]time.Time)}
}

// isDuplicate 检查事件是否为重复事件，返回 true 表示应跳过
func (d *eventDeduper) isDuplicate(event *LoginEvent) bool {
	// 只对失败事件去重（成功事件不会重复）
	if event.Type != EventLoginFailed {
		return false
	}
	key := fmt.Sprintf("%s:%d:%s", event.IP, event.Port, event.User)
	if lastSeen, ok := d.seen[key]; ok {
		if event.Timestamp.Sub(lastSeen) < eventDedupeWindow {
			return true
		}
	}
	d.seen[key] = event.Timestamp
	// 清理过期条目（简单实现，避免内存泄漏）
	for k, t := range d.seen {
		if event.Timestamp.Sub(t) > eventDedupeWindow*10 {
			delete(d.seen, k)
		}
	}
	return false
}

// failRateLimiter 限制每个 IP 在时间窗口内的失败通知数量
type failRateLimiter struct {
	limit  int           // 限制数量，0 表示不限制
	window time.Duration // 时间窗口
	counts map[string][]time.Time
}

func newFailRateLimiter(limit int, window time.Duration) *failRateLimiter {
	return &failRateLimiter{
		limit:  limit,
		window: window,
		counts: make(map[string][]time.Time),
	}
}

// shouldLimit 检查是否应该限制该失败事件的通知，返回 true 表示应跳过
func (r *failRateLimiter) shouldLimit(event *LoginEvent) bool {
	if r.limit <= 0 || event.Type != EventLoginFailed {
		return false
	}

	now := event.Timestamp
	ip := event.IP

	// 清理过期记录
	if times, ok := r.counts[ip]; ok {
		var valid []time.Time
		for _, t := range times {
			if now.Sub(t) < r.window {
				valid = append(valid, t)
			}
		}
		r.counts[ip] = valid
	}

	// 检查是否超过限制
	if len(r.counts[ip]) >= r.limit {
		debugf("notify: 失败通知限流，IP=%s 在 %v 内已达 %d 次", ip, r.window, r.limit)
		return true
	}

	// 记录本次
	r.counts[ip] = append(r.counts[ip], now)

	// 定期清理长时间无活动的 IP
	for k, times := range r.counts {
		if len(times) == 0 || now.Sub(times[len(times)-1]) > r.window*2 {
			delete(r.counts, k)
		}
	}

	return false
}

// notifyFilter 组合去重和限流逻辑
type notifyFilter struct {
	notifyOn    NotifyOn
	deduper     *eventDeduper
	rateLimiter *failRateLimiter
}

func newNotifyFilter(notifyOn NotifyOn, failLimit int, failWindow time.Duration) *notifyFilter {
	return &notifyFilter{
		notifyOn:    notifyOn,
		deduper:     newEventDeduper(),
		rateLimiter: newFailRateLimiter(failLimit, failWindow),
	}
}

// shouldNotify 检查是否应该发送通知
func (f *notifyFilter) shouldNotify(event *LoginEvent) bool {
	// 检查通知类型过滤
	switch f.notifyOn {
	case NotifyOnSuccess:
		if event.Type != EventLoginSuccess {
			return false
		}
	case NotifyOnFailed:
		if event.Type != EventLoginFailed {
			return false
		}
	}

	// 检查去重
	if f.deduper.isDuplicate(event) {
		debugf("notify: 跳过重复事件 %s@%s:%d", event.User, event.IP, event.Port)
		return false
	}

	// 检查限流
	if f.rateLimiter.shouldLimit(event) {
		return false
	}

	return true
}

// 解析 systemd journal 输出（journalctl -o json）的结构体
type journalRecord struct {
	Cursor     string `json:"__CURSOR"`
	Message    string `json:"MESSAGE"`
	Hostname   string `json:"_HOSTNAME"`
	RealtimeTS string `json:"__REALTIME_TIMESTAMP"`
	Unit       string `json:"_SYSTEMD_UNIT"`
}

// WatchOptions 控制 watch 模式行为
type WatchOptions struct {
	CursorPath   string
	Source       string
	JournalUnits []string
	LogPaths     []string
	PollTimeout  time.Duration
	DisplayLoc   *time.Location
	NotifyOn     NotifyOn      // 通知类型：all/success/failed
	FailLimit    int           // 每 IP 失败通知限制数量，0 表示不限制
	FailWindow   time.Duration // 失败限制时间窗口
}

// SweepOptions 控制 sweep 模式行为
type SweepOptions struct {
	CursorPath   string
	Source       string
	JournalUnits []string
	LogPaths     []string
	Since        time.Duration
	Notify       bool
	DisplayLoc   *time.Location
	NotifyOn     NotifyOn      // 通知类型：all/success/failed
	FailLimit    int           // 每 IP 失败通知限制数量，0 表示不限制
	FailWindow   time.Duration // 失败限制时间窗口
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
	if opts.NotifyOn == "" {
		opts.NotifyOn = NotifyOnAll
	}

	store, err := NewCursorStore(opts.CursorPath)
	if err != nil {
		return err
	}

	state, err := store.Load()
	if err != nil {
		return err
	}

	selection, err := determineSource(opts.Source, opts.JournalUnits, opts.LogPaths, state, 0, true)
	if err != nil {
		return err
	}

	fmt.Printf(">>> 监听模式：%s\n", selection.Description)
	if opts.NotifyOn != NotifyOnAll {
		fmt.Printf(">>> 通知过滤：%s\n", opts.NotifyOn)
	}
	if opts.FailLimit > 0 {
		fmt.Printf(">>> 失败限流：每 IP %d 次 / %v\n", opts.FailLimit, opts.FailWindow)
	}
	loc := normalizeLocation(opts.DisplayLoc)
	filter := newNotifyFilter(opts.NotifyOn, opts.FailLimit, opts.FailWindow)

	switch selection.Source {
	case sourceJournal:
		return runJournalWithFilter(ctx, store, state, selection.Units, opts.PollTimeout, true, 0, loc, true, filter)
	case sourceFile:
		return followLogFileWithFilter(ctx, store, state, selection.LogPath, opts.PollTimeout, loc, filter)
	default:
		return fmt.Errorf("未知监听源: %s", selection.Source)
	}
}

// RunSweep 处理近期 SSH 登录事件后退出
func RunSweep(ctx context.Context, opts SweepOptions) error {
	if opts.NotifyOn == "" {
		opts.NotifyOn = NotifyOnAll
	}

	store, err := NewCursorStore(opts.CursorPath)
	if err != nil {
		return err
	}

	state, err := store.Load()
	if err != nil {
		return err
	}

	selection, err := determineSource(opts.Source, opts.JournalUnits, opts.LogPaths, state, opts.Since, false)
	if err != nil {
		return err
	}

	fmt.Printf(">>> 扫描模式：%s\n", selection.Description)
	if opts.Notify && opts.NotifyOn != NotifyOnAll {
		fmt.Printf(">>> 通知过滤：%s\n", opts.NotifyOn)
	}
	if opts.Notify && opts.FailLimit > 0 {
		fmt.Printf(">>> 失败限流：每 IP %d 次 / %v\n", opts.FailLimit, opts.FailWindow)
	}
	loc := normalizeLocation(opts.DisplayLoc)
	var filter *notifyFilter
	if opts.Notify {
		filter = newNotifyFilter(opts.NotifyOn, opts.FailLimit, opts.FailWindow)
	}

	switch selection.Source {
	case sourceJournal:
		return runJournalWithFilter(ctx, store, state, selection.Units, 0, false, opts.Since, loc, opts.Notify, filter)
	case sourceFile:
		return sweepLogFileWithFilter(ctx, store, state, selection.LogPath, opts.Since, loc, opts.Notify, filter)
	default:
		return fmt.Errorf("未知监听源: %s", selection.Source)
	}
}

func determineSource(source string, units, paths []string, state *SourceState, since time.Duration, follow bool) (*sourceSelection, error) {
	s := &sourceSelection{}

	if len(units) == 0 {
		units = append([]string{}, defaultJournalUnits...)
	}
	if len(paths) == 0 {
		paths = append([]string{}, defaultLogPaths...)
	}

	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = sourceAuto
	}

	journalOK, journalCount := probeJournal(units, state, since)
	journalRecent := journalCount > 0
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
		if follow {
			if journalOK {
				s.Source = sourceJournal
				s.Units = units
				desc := fmt.Sprintf("journald（units=%v）", units)
				if !journalRecent {
					desc += "，等待新事件"
				}
				s.Description = desc
				return s, nil
			}
			if logExists {
				s.Source = sourceFile
				s.LogPath = logPath
				s.Description = fmt.Sprintf("文件日志：%s", logPath)
				return s, nil
			}
		} else {
			if journalOK && journalRecent {
				s.Source = sourceJournal
				s.Units = units
				s.Description = fmt.Sprintf("journald（units=%v，命中近期事件）", units)
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
				s.Description = fmt.Sprintf("journald（units=%v，无匹配事件）", units)
				return s, nil
			}
		}
		return nil, fmt.Errorf("未检测到可用的日志源（journalctl 不可用，且未找到 %v）", paths)
	default:
		return nil, fmt.Errorf("不支持的 source：%s（可选 auto|journal|file）", source)
	}
}

func probeJournal(units []string, state *SourceState, since time.Duration) (bool, int) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		debugf("notify: journalctl not found")
		return false, 0
	}

	args := []string{"--no-pager", "-n", "200", "-o", "json"}
	if state != nil && state.JournalCursor != "" && since <= 0 {
		args = append(args, "--after-cursor", state.JournalCursor)
	} else {
		window := since
		if window <= 0 {
			window = 30 * time.Minute
		}
		t := time.Now().Add(-window).Format("2006-01-02 15:04:05")
		args = append(args, "--since", t)
	}
	for _, unit := range units {
		args = append(args, "-u", unit)
	}

	debugf("notify: probeJournal cmd: journalctl %v", args)

	cmd := exec.Command("journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		debugf("notify: probeJournal stdout pipe error: %v", err)
		return false, 0
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		debugf("notify: probeJournal start error: %v", err)
		return false, 0
	}

	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	count := 0
	totalLines := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		totalLines++
		var record journalRecord
		if err := json.Unmarshal(line, &record); err != nil {
			debugf("notify: probeJournal json parse error: %v", err)
			continue
		}
		ts := parseRealtime(record.RealtimeTS)
		debugf("notify: probeJournal message: %s", record.Message)
		if _, ok := parseJournalMessage(record.Message, record.Hostname, ts); ok {
			count++
			break
		}
	}
	_ = cmd.Wait()
	if stderr.Len() > 0 {
		debugf("notify: probeJournal stderr: %s", stderr.String())
	}
	debugf("notify: probeJournal result: totalLines=%d, matchedCount=%d", totalLines, count)
	return true, count
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

func runJournalWithFilter(ctx context.Context, store *CursorStore, state *SourceState, units []string, poll time.Duration, follow bool, since time.Duration, loc *time.Location, notify bool, filter *notifyFilter) error {
	if state == nil {
		state = &SourceState{}
	}

	startTime := time.Now()
	skipHistorical := follow && state.JournalCursor == "" && since <= 0
	// 如果没有 filter，创建一个默认的（兼容旧调用）
	if filter == nil {
		filter = newNotifyFilter(NotifyOnAll, 0, 0)
	}

	args := []string{"--no-pager", "-o", "json"}
	if follow {
		args = append(args, "--follow")
	}
	for _, unit := range units {
		args = append(args, "-u", unit)
	}

	if !follow && since > 0 {
		sinceTime := time.Now().Add(-since).Format("2006-01-02 15:04:05")
		args = append(args, "--since", sinceTime)
	} else if state.JournalCursor != "" {
		args = append(args, "--after-cursor", state.JournalCursor)
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

		if skipHistorical && shouldSkipHistoricalEvent(startTime, ts) {
			skipHistorical = true
			debugf("notify: 跳过历史 journald 事件 cursor=%s ts=%s", record.Cursor, ts.Format(time.RFC3339))
			state.JournalCursor = record.Cursor
			if err := store.Save(state); err != nil {
				log.Printf("写入状态失败: %v", err)
			}
			continue
		}
		skipHistorical = false

		logSource := record.Unit
		if logSource == "" {
			logSource = strings.Join(units, ",")
		}
		if logSource != "" {
			event.LogPath = fmt.Sprintf("journald:%s", logSource)
		} else {
			event.LogPath = "journald"
		}

		// 使用 filter 检查是否应该发送通知
		shouldSend := notify && filter.shouldNotify(event)
		if notify && !shouldSend {
			// 更新游标但不发送通知
			state.JournalCursor = record.Cursor
			if err := store.Save(state); err != nil {
				log.Printf("写入状态失败: %v", err)
			}
			continue
		}

		if shouldSend {
			if err := dispatchEvent(event); err != nil {
				log.Printf("发送通知失败: %v", err)
			}
		}

		printEventSummary(*event, loc)

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

func shouldSkipHistoricalEvent(start, event time.Time) bool {
	return event.Before(start.Add(-journalHistoryTolerance))
}

func followLogFileWithFilter(ctx context.Context, store *CursorStore, state *SourceState, path string, poll time.Duration, loc *time.Location, filter *notifyFilter) error {
	if poll <= 0 {
		poll = time.Second
	}

	offset := state.FileOffsets[path]
	if offset < 0 {
		offset = 0
	}

	if filter == nil {
		filter = newNotifyFilter(NotifyOnAll, 0, 0)
	}

	process := func(event *LoginEvent, newOffset int64) {
		event.LogPath = path
		// 使用 filter 检查是否应该发送通知
		if !filter.shouldNotify(event) {
			offset = newOffset
			state.FileOffsets[path] = offset
			if err := store.Save(state); err != nil {
				log.Printf("写入状态失败: %v", err)
			}
			return
		}
		if err := dispatchEvent(event); err != nil {
			log.Printf("发送通知失败: %v", err)
		}
		printEventSummary(*event, loc)
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

func sweepLogFileWithFilter(ctx context.Context, store *CursorStore, state *SourceState, path string, since time.Duration, loc *time.Location, notify bool, filter *notifyFilter) error {
	offset := state.FileOffsets[path]
	startOffset := offset
	cutoff := time.Time{}
	if since > 0 {
		startOffset = 0
		cutoff = time.Now().Add(-since)
	}

	if filter == nil {
		filter = newNotifyFilter(NotifyOnAll, 0, 0)
	}

	latest := offset
	process := func(event *LoginEvent, newOffset int64) {
		if newOffset > latest {
			latest = newOffset
		}
		if !cutoff.IsZero() && event.Timestamp.Before(cutoff) {
			return
		}
		event.LogPath = path
		// 使用 filter 检查是否应该发送通知
		shouldSend := notify && filter.shouldNotify(event)
		if shouldSend {
			if err := dispatchEvent(event); err != nil {
				log.Printf("发送通知失败: %v", err)
			}
		}
		printEventSummary(*event, loc)
	}

	finalOffset, err := readLogFile(ctx, path, startOffset, process, false, 0)
	if err != nil {
		return err
	}
	if finalOffset > latest {
		latest = finalOffset
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
	if cfg == nil {
		return nil
	}

	channels := cfg.GetEnabledChannels()
	if len(channels) == 0 {
		return nil
	}

	var errs []error
	for _, ch := range channels {
		notifier, err := buildChannelNotifier(ch)
		if err != nil {
			errs = append(errs, fmt.Errorf("渠道 %s: %w", channelDisplayName(ch), err))
			continue
		}
		if err := notifier.Send(*event); err != nil {
			errs = append(errs, fmt.Errorf("渠道 %s: %w", channelDisplayName(ch), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("部分通知失败: %v", errs)
	}
	return nil
}

func normalizeLocation(loc *time.Location) *time.Location {
	if loc == nil {
		return shanghaiLocation
	}
	return loc
}

// buildChannelNotifier 根据渠道配置构建通知器
func buildChannelNotifier(ch ChannelConfig) (Notifier, error) {
	switch strings.ToLower(ch.Type) {
	case "curl":
		if ch.Curl == nil {
			return nil, fmt.Errorf("curl 配置为空")
		}
		return NewCurlNotifier(ch.Curl.Command)
	case "email":
		if ch.Email == nil {
			return nil, fmt.Errorf("email 配置为空")
		}
		return NewEmailNotifierFromChannel(ch.Email), nil
	default:
		return nil, fmt.Errorf("未知通知类型: %s", ch.Type)
	}
}

func printEventSummary(event LoginEvent, loc *time.Location) {
	if loc == nil {
		loc = shanghaiLocation
	}

	method := event.Method
	if method == "" {
		method = "-"
	}

	port := "-"
	if event.Port > 0 {
		port = fmt.Sprintf("%d", event.Port)
	}

	displayTime := event.Timestamp.In(loc).Format("2006-01-02 15:04:05 -07:00")
	logPath := event.LogPath
	if strings.TrimSpace(logPath) == "" {
		logPath = "-"
	}

	fmt.Fprintf(os.Stdout, "[%s] %s 用户=%s IP=%s 端口=%s 方式=%s 主机=%s 日志路径=%s\n",
		displayTime,
		event.Type,
		event.User,
		event.IP,
		port,
		method,
		event.Hostname,
		logPath,
	)
}
