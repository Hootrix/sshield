package notify

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	successRe = regexp.MustCompile(`^Accepted (\S+) for (\S+) from ([^ ]+) port (\d+)`)
	failRe    = regexp.MustCompile(`^Failed (\S+) for (?:invalid user )?(\S+) from ([^ ]+) port (\d+)`)
	syslogRe  = regexp.MustCompile(`^(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+(\d{1,2})\s+(\d{2}:\d{2}:\d{2})\s+([^ ]+)\s+sshd(?:\[[^]]*\])?:\s+(.*)$`)
)

func parseJournalMessage(message, host string, ts time.Time) (*LoginEvent, bool) {
	if message == "" {
		return nil, false
	}

	if matches := successRe.FindStringSubmatch(message); len(matches) == 5 {
		port, _ := strconv.Atoi(matches[4])
		ip := stripAddress(matches[3])
		return &LoginEvent{
			Type:      EventLoginSuccess,
			User:      matches[2],
			IP:        ip,
			Method:    normalizeMethod(matches[1]),
			Port:      port,
			Timestamp: ts,
			Hostname:  host,
			Message:   message,
			Location:  LookupIPLocation(ip),
		}, true
	}

	if matches := failRe.FindStringSubmatch(message); len(matches) == 5 {
		port, _ := strconv.Atoi(matches[4])
		ip := stripAddress(matches[3])
		return &LoginEvent{
			Type:      EventLoginFailed,
			User:      matches[2],
			IP:        ip,
			Method:    normalizeMethod(matches[1]),
			Port:      port,
			Timestamp: ts,
			Hostname:  host,
			Message:   message,
			Location:  LookupIPLocation(ip),
		}, true
	}

	return nil, false
}

func normalizeMethod(method string) string {
	method = strings.ToLower(method)
	switch method {
	case "publickey":
		return "publickey"
	case "password":
		return "password"
	case "keyboard-interactive/pam", "keyboard-interactive":
		return "keyboard-interactive"
	default:
		return method
	}
}

func stripAddress(addr string) string {
	addr = strings.TrimPrefix(addr, "::ffff:")
	if idx := strings.Index(addr, "%"); idx != -1 {
		addr = addr[:idx]
	}
	return addr
}

func parseAuthLogLine(line string) (*LoginEvent, bool) {
	if line == "" {
		return nil, false
	}

	matches := syslogRe.FindStringSubmatch(line)
	if len(matches) != 6 {
		return nil, false
	}

	month := matches[1]
	day, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, false
	}
	clock := matches[3]
	host := matches[4]
	message := matches[5]

	now := time.Now()
	layout := "Jan 2 15:04:05 2006"
	timestampStr := fmt.Sprintf("%s %d %s %d", month, day, clock, now.Year())
	ts, err := time.ParseInLocation(layout, timestampStr, shanghaiLocation)
	if err != nil {
		return nil, false
	}

	// 处理跨年日志：如果解析结果在未来较远时间，向前调整一年
	if ts.After(now.Add(24 * time.Hour)) {
		ts = ts.AddDate(-1, 0, 0)
	}

	return parseJournalMessage(message, host, ts)
}
