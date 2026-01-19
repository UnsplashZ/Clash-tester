package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type MihomoCore struct {
	BinaryPath string
	ConfigPath string
	Port       int
	APIPort    int
	cmd        *exec.Cmd
}

func NewMihomoCore(binaryPath, configPath string, port, apiPort int) *MihomoCore {
	return &MihomoCore{
		BinaryPath: binaryPath,
		ConfigPath: configPath,
		Port:       port,
		APIPort:    apiPort,
	}
}

// Start 启动mihomo核心
func (m *MihomoCore) Start() error {
	// 确保配置文件路径是绝对路径
	absConfigPath, err := filepath.Abs(m.ConfigPath)
	if err != nil {
		return err
	}

	// 检查二进制文件是否存在
	if _, err := os.Stat(m.BinaryPath); os.IsNotExist(err) {
		// 尝试从当前目录查找
		if _, err := os.Stat("mihomo.exe"); err == nil {
			m.BinaryPath = "mihomo.exe"
		} else {
			return fmt.Errorf("mihomo binary not found at %s", m.BinaryPath)
		}
	}

	// Fix: Convert binary path to absolute path to satisfy Go's security checks on Windows
	if absBinaryPath, err := filepath.Abs(m.BinaryPath); err == nil {
		m.BinaryPath = absBinaryPath
	}

	// Windows下通常是mihomo.exe，确保路径正确
	m.cmd = exec.Command(m.BinaryPath, "-f", absConfigPath, "-d", filepath.Dir(absConfigPath))

	// 重定向输出以便调试（可选，或者设为nil忽略）
	// m.cmd.Stdout = os.Stdout
	// m.cmd.Stderr = os.Stderr

	if err := m.cmd.Start(); err != nil {
		return err
	}

	// 等待核心启动
	// 这里可以优化为轮询检测API端口是否通
	for i := 0; i < 20; i++ { // 增加等待时间，因为并发启动可能慢
		if m.checkHealth() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("mihomo failed to start within timeout")
}

func (m *MihomoCore) checkHealth() bool {
	url := fmt.Sprintf("http://127.0.0.1:%d", m.APIPort)
	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}

// SwitchProxy 切换代理节点
func (m *MihomoCore) SwitchProxy(proxyName string) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/proxies/GLOBAL", m.APIPort)

	data := map[string]string{"name": proxyName}
	jsonData, _ := json.Marshal(data)

	req, _ := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		return fmt.Errorf("failed to switch proxy: %d", resp.StatusCode)
	}

	return nil
}

// Stop 停止mihomo核心
func (m *MihomoCore) Stop() error {
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Kill()
	}
	return nil
}

// GetProxyURL 获取代理地址
func (m *MihomoCore) GetProxyURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", m.Port)
}