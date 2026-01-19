package config

import (
	"Clash-tester/pkg/models"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

// GenerateMihomoConfig 为测试生成mihomo配置
func GenerateMihomoConfig(nodes []models.ProxyNode, outputPath string, port, apiPort int) error {
	config := map[string]interface{}{
		"port":                port,
		"socks-port":          port + 1,
		"allow-lan":           false,
		"mode":                "global",
		"log-level":           "silent",
		"external-controller": fmt.Sprintf("127.0.0.1:%d", apiPort),
		"proxies":             nodes,
		"proxy-groups": []map[string]interface{}{
			{
				"name":    "GLOBAL",
				"type":    "select",
				"proxies": getNodeNames(nodes),
			},
		},
		// 必须有一些基本规则，虽然 GLOBAL 模式下主要是走 GLOBAL 组
		"rules": []string{
			"MATCH,GLOBAL",
		},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, data, 0644)
}

func getNodeNames(nodes []models.ProxyNode) []string {
	names := make([]string, len(nodes))
	for i, node := range nodes {
		names[i] = node.Name
	}
	return names
}