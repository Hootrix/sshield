package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// IPLookupResult IP 查询结果
type IPLookupResult struct {
	Country string
	Region  string
	City    string
}

func (r *IPLookupResult) String() string {
	parts := []string{}
	if r.Country != "" {
		parts = append(parts, r.Country)
	}
	if r.Region != "" && r.Region != r.City {
		parts = append(parts, r.Region)
	}
	if r.City != "" {
		parts = append(parts, r.City)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

// IPLookupProvider IP 查询提供者
type IPLookupProvider interface {
	Name() string
	Lookup(ctx context.Context, ip string) (*IPLookupResult, error)
}

// ipinfoProvider ipinfo.io 提供者
type ipinfoProvider struct {
	client *http.Client
}

func (p *ipinfoProvider) Name() string {
	return "ipinfo.io"
}

func (p *ipinfoProvider) Lookup(ctx context.Context, ip string) (*IPLookupResult, error) {
	url := fmt.Sprintf("https://ipinfo.io/%s/json", ip)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		Country string `json:"country"`
		Region  string `json:"region"`
		City    string `json:"city"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	return &IPLookupResult{
		Country: data.Country,
		Region:  data.Region,
		City:    data.City,
	}, nil
}

// ipApiProvider ip-api.com 提供者
type ipApiProvider struct {
	client *http.Client
}

func (p *ipApiProvider) Name() string {
	return "ip-api.com"
}

func (p *ipApiProvider) Lookup(ctx context.Context, ip string) (*IPLookupResult, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,regionName,city", ip)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		Status     string `json:"status"`
		Country    string `json:"country"`
		RegionName string `json:"regionName"`
		City       string `json:"city"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	if data.Status != "success" {
		return nil, fmt.Errorf("lookup failed")
	}

	return &IPLookupResult{
		Country: data.Country,
		Region:  data.RegionName,
		City:    data.City,
	}, nil
}

// IPLookup IP 地理位置查询器
type IPLookup struct {
	providers []IPLookupProvider
	cache     sync.Map // ip -> *IPLookupResult
	timeout   time.Duration
}

var defaultIPLookup *IPLookup
var ipLookupOnce sync.Once

// GetIPLookup 获取全局 IP 查询器
func GetIPLookup() *IPLookup {
	ipLookupOnce.Do(func() {
		client := &http.Client{Timeout: 5 * time.Second}
		defaultIPLookup = &IPLookup{
			providers: []IPLookupProvider{
				&ipinfoProvider{client: client},
				&ipApiProvider{client: client},
			},
			timeout: 5 * time.Second,
		}
	})
	return defaultIPLookup
}

// Lookup 查询 IP 地理位置，带缓存和多提供者回退
func (l *IPLookup) Lookup(ip string) string {
	// 跳过内网 IP
	if isPrivateIP(ip) {
		return "内网"
	}

	// 检查缓存
	if cached, ok := l.cache.Load(ip); ok {
		return cached.(*IPLookupResult).String()
	}

	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()

	// 依次尝试各提供者
	for _, provider := range l.providers {
		result, err := provider.Lookup(ctx, ip)
		if err != nil {
			debugf("notify: IP 查询失败 provider=%s ip=%s err=%v", provider.Name(), ip, err)
			continue
		}

		if result != nil && result.String() != "" {
			l.cache.Store(ip, result)
			return result.String()
		}
	}

	// 所有提供者都失败，缓存空结果避免重复查询
	l.cache.Store(ip, &IPLookupResult{})
	return ""
}

// isPrivateIP 判断是否为内网 IP
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// 本地回环
	if ip.IsLoopback() {
		return true
	}

	// 私有地址段
	privateBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7",  // IPv6 私有地址
		"fe80::/10", // IPv6 链路本地
	}

	for _, block := range privateBlocks {
		_, cidr, err := net.ParseCIDR(block)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}

// LookupIPLocation 便捷函数
func LookupIPLocation(ip string) string {
	return GetIPLookup().Lookup(ip)
}
