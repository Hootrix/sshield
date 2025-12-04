package notify

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
)

// CurlRequest 解析后的 curl 请求
type CurlRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

// ParseCurl 解析 curl 命令字符串
// 支持: -X/--request, -H/--header, -d/--data/--data-raw, URL
func ParseCurl(curlCmd string) (*CurlRequest, error) {
	args, err := tokenize(curlCmd)
	if err != nil {
		return nil, err
	}

	req := &CurlRequest{
		Method:  "GET",
		Headers: make(map[string]string),
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "curl":
			continue

		case arg == "-X" || arg == "--request":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for %s", arg)
			}
			i++
			req.Method = strings.ToUpper(args[i])

		case arg == "-H" || arg == "--header":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for %s", arg)
			}
			i++
			key, value, ok := parseHeader(args[i])
			if !ok {
				return nil, fmt.Errorf("invalid header format: %s", args[i])
			}
			req.Headers[key] = value

		case arg == "-d" || arg == "--data" || arg == "--data-raw":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for %s", arg)
			}
			i++
			req.Body = args[i]
			if req.Method == "GET" {
				req.Method = "POST"
			}

		case strings.HasPrefix(arg, "-"):
			// 跳过不支持的参数
			// 如果下一个参数不是以 - 开头，可能是该参数的值，也跳过
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && !isURL(args[i+1]) {
				i++
			}

		default:
			// 假设是 URL
			if isURL(arg) {
				req.URL = arg
			}
		}
	}

	if req.URL == "" {
		return nil, fmt.Errorf("no URL found in curl command")
	}

	return req, nil
}

// Execute 执行请求，支持模板变量替换
func (r *CurlRequest) Execute(data any) (*http.Response, error) {
	// 渲染 URL
	url, err := renderTemplate(r.URL, data)
	if err != nil {
		return nil, fmt.Errorf("failed to render URL template: %w", err)
	}

	// 渲染 Body
	body, err := renderTemplate(r.Body, data)
	if err != nil {
		return nil, fmt.Errorf("failed to render body template: %w", err)
	}

	// 创建请求
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(r.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	for key, value := range r.Headers {
		rendered, err := renderTemplate(value, data)
		if err != nil {
			return nil, fmt.Errorf("failed to render header %s: %w", key, err)
		}
		req.Header.Set(key, rendered)
	}

	// 默认 Content-Type
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	return http.DefaultClient.Do(req)
}

// tokenize 将 curl 命令分割成 token，处理引号
func tokenize(cmd string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	var inQuote rune
	var escape bool

	for _, ch := range cmd {
		if escape {
			current.WriteRune(ch)
			escape = false
			continue
		}

		if ch == '\\' {
			escape = true
			continue
		}

		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			} else {
				current.WriteRune(ch)
			}
			continue
		}

		switch ch {
		case '"', '\'':
			inQuote = ch
		case ' ', '\t', '\n', '\r':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	if inQuote != 0 {
		return nil, fmt.Errorf("unclosed quote in curl command")
	}

	return tokens, nil
}

// parseHeader 解析 "Key: Value" 格式的 header
func parseHeader(h string) (key, value string, ok bool) {
	idx := strings.Index(h, ":")
	if idx == -1 {
		return "", "", false
	}
	return strings.TrimSpace(h[:idx]), strings.TrimSpace(h[idx+1:]), true
}

// isURL 简单判断是否是 URL
func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// renderTemplate 渲染模板字符串
func renderTemplate(text string, data any) (string, error) {
	if data == nil || !strings.Contains(text, "{{") {
		return text, nil
	}

	tmpl, err := template.New("").Parse(text)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
