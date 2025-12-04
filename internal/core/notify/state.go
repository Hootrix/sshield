package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// 存储读取进度的文件
const cursorFileName = "notify.state"

// SourceState 记录不同来源的处理进度
type SourceState struct {
	JournalCursor string           `json:"journal_cursor,omitempty"`
	FileOffsets   map[string]int64 `json:"file_offsets,omitempty"`
}

// CursorStore 管理状态持久化
type CursorStore struct {
	path string
}

// NewCursorStore 创建游标管理器
func NewCursorStore(path string) (*CursorStore, error) {
	if path == "" {
		return nil, errors.New("cursor path is required")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cursor directory: %w", err)
	}

	return &CursorStore{path: path}, nil
}

// Load 读取状态
func (c *CursorStore) Load() (*SourceState, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SourceState{
				FileOffsets: make(map[string]int64),
			}, nil
		}
		return nil, fmt.Errorf("failed to read cursor: %w", err)
	}

	if len(data) == 0 {
		return &SourceState{
			FileOffsets: make(map[string]int64),
		}, nil
	}

	state := &SourceState{}
	if err := json.Unmarshal(data, state); err != nil {
		// 兼容旧版本：纯字符串表示 journald cursor
		state.JournalCursor = string(data)
	}
	if state.FileOffsets == nil {
		state.FileOffsets = make(map[string]int64)
	}
	return state, nil
}

// Save 持久化状态
func (c *CursorStore) Save(state *SourceState) error {
	if state == nil {
		return nil
	}
	if state.FileOffsets == nil {
		state.FileOffsets = make(map[string]int64)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cursor state: %w", err)
	}

	return os.WriteFile(c.path, data, 0600)
}

// defaultCursorPath 根据身份选择游标存储位置
func defaultCursorPath() (string, error) {
	if os.Geteuid() == 0 {
		if err := os.MkdirAll(defaultStateRoot, 0700); err != nil {
			return "", fmt.Errorf("failed to create state directory: %w", err)
		}
		return filepath.Join(defaultStateRoot, cursorFileName), nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("failed to resolve state directory: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}

	path := filepath.Join(configDir, "sshield", cursorFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("failed to create user state directory: %w", err)
	}
	return path, nil
}
