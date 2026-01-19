package parser

import (
	"Clash-tester/pkg/models"
	"gopkg.in/yaml.v3"
)

type ClashConfig struct {
	Proxies []models.ProxyNode `yaml:"proxies"`
}

// Parse 解析Clash配置
func Parse(data []byte) ([]models.ProxyNode, error) {
	var config ClashConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// 过滤支持的协议
	var supported []models.ProxyNode
	for _, proxy := range config.Proxies {
		if isSupportedProtocol(proxy.Type) {
			supported = append(supported, proxy)
		}
	}

	return supported, nil
}

// isSupportedProtocol 检查是否为支持的协议
func isSupportedProtocol(protocol string) bool {
	supported := map[string]bool{
		"hysteria2": true,
		"trojan":    true,
		"vless":     true,
		"ss":        true,
		"vmess":     true, // Added vmess as it is very common
	}
	return supported[protocol]
}
