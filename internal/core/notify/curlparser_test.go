package notify

import (
	"testing"
)

func TestParseCurl(t *testing.T) {
	tests := []struct {
		name    string
		curl    string
		want    *CurlRequest
		wantErr bool
	}{
		{
			name: "simple GET",
			curl: `curl https://example.com/api`,
			want: &CurlRequest{
				Method:  "GET",
				URL:     "https://example.com/api",
				Headers: map[string]string{},
			},
		},
		{
			name: "POST with data",
			curl: `curl -X POST -d '{"msg": "hello"}' https://example.com/api`,
			want: &CurlRequest{
				Method:  "POST",
				URL:     "https://example.com/api",
				Headers: map[string]string{},
				Body:    `{"msg": "hello"}`,
			},
		},
		{
			name: "POST with headers",
			curl: `curl -X POST -H "Content-Type: application/json" -H "Authorization: Bearer token123" -d '{"user": "test"}' https://example.com/webhook`,
			want: &CurlRequest{
				Method: "POST",
				URL:    "https://example.com/webhook",
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Authorization": "Bearer token123",
				},
				Body: `{"user": "test"}`,
			},
		},
		{
			name: "implicit POST with -d",
			curl: `curl -d "data=value" https://example.com/api`,
			want: &CurlRequest{
				Method:  "POST",
				URL:     "https://example.com/api",
				Headers: map[string]string{},
				Body:    "data=value",
			},
		},
		{
			name: "with template variables",
			curl: `curl -X POST -H "Content-Type: application/json" -d '{"user": "{{.User}}", "ip": "{{.IP}}"}' https://example.com/webhook`,
			want: &CurlRequest{
				Method: "POST",
				URL:    "https://example.com/webhook",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{"user": "{{.User}}", "ip": "{{.IP}}"}`,
			},
		},
		{
			name: "multiline with backslash",
			curl: `curl -X POST \
				-H "Content-Type: application/json" \
				-d '{"msg": "test"}' \
				https://example.com/api`,
			want: &CurlRequest{
				Method: "POST",
				URL:    "https://example.com/api",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{"msg": "test"}`,
			},
		},
		{
			name:    "no URL",
			curl:    `curl -X POST -d "data"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCurl(tt.curl)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCurl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if got.Method != tt.want.Method {
				t.Errorf("Method = %v, want %v", got.Method, tt.want.Method)
			}
			if got.URL != tt.want.URL {
				t.Errorf("URL = %v, want %v", got.URL, tt.want.URL)
			}
			if got.Body != tt.want.Body {
				t.Errorf("Body = %v, want %v", got.Body, tt.want.Body)
			}
			for k, v := range tt.want.Headers {
				if got.Headers[k] != v {
					t.Errorf("Header[%s] = %v, want %v", k, got.Headers[k], v)
				}
			}
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	data := map[string]string{
		"User": "root",
		"IP":   "192.168.1.1",
	}

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{
			name:     "simple variable",
			template: `{"user": "{{.User}}"}`,
			want:     `{"user": "root"}`,
		},
		{
			name:     "multiple variables",
			template: `{"user": "{{.User}}", "ip": "{{.IP}}"}`,
			want:     `{"user": "root", "ip": "192.168.1.1"}`,
		},
		{
			name:     "no template",
			template: `{"msg": "hello"}`,
			want:     `{"msg": "hello"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderTemplate(tt.template, data)
			if err != nil {
				t.Errorf("renderTemplate() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("renderTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		want    []string
		wantErr bool
	}{
		{
			name: "simple",
			cmd:  `curl https://example.com`,
			want: []string{"curl", "https://example.com"},
		},
		{
			name: "with double quotes",
			cmd:  `curl -H "Content-Type: application/json" https://example.com`,
			want: []string{"curl", "-H", "Content-Type: application/json", "https://example.com"},
		},
		{
			name: "with single quotes",
			cmd:  `curl -d '{"key": "value"}' https://example.com`,
			want: []string{"curl", "-d", `{"key": "value"}`, "https://example.com"},
		},
		{
			name:    "unclosed quote",
			cmd:     `curl -d "unclosed`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tokenize(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("tokenize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("tokenize() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("tokenize()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
