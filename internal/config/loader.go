package config

import (
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type LoaderConfig struct {
	Source  string // URL或文件路径
	Timeout int    // 超时时间（秒）
}

// Load 加载配置（自动判断在线/本地）
func Load(cfg LoaderConfig) ([]byte, error) {
	if strings.HasPrefix(cfg.Source, "http://") ||
		strings.HasPrefix(cfg.Source, "https://") {
		return loadFromURL(cfg.Source, cfg.Timeout)
	}
	return loadFromFile(cfg.Source)
}

// loadFromURL 从在线订阅加载
func loadFromURL(url string, timeout int) ([]byte, error) {
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 尝试base64解码
	// 很多订阅链接返回的是base64编码的内容，但也有直接返回yaml的
	// 这里简单尝试一下，如果失败就返回原始内容
	// 注意：实际情况中可能需要更严谨的判断
	if decoded, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
		return decoded, nil
	}

	return data, nil
}

// loadFromFile 从本地文件加载
func loadFromFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
