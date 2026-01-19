package reporter

import (
	"Clash-tester/pkg/models"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// NodeTagData 定义了输出给 SubStore 使用的精简数据结构
type NodeTagData struct {
	UpdateTime time.Time             `json:"update_time"`
	OpenAI     *models.ServiceTest   `json:"openai,omitempty"`
	Gemini     *models.ServiceTest   `json:"gemini,omitempty"`
	Claude     *models.ServiceTest   `json:"claude,omitempty"`
	Netflix    *StreamTagData        `json:"netflix,omitempty"`
	Disney     *StreamTagData        `json:"disney,omitempty"`
	Youtube    *StreamTagData        `json:"youtube,omitempty"`
	Max        *StreamTagData        `json:"max,omitempty"`
}

type StreamTagData struct {
	Available bool   `json:"available"`
	Region    string `json:"region,omitempty"`
	Result    string `json:"result,omitempty"` // For Netflix: Full / Originals
	Premium   bool   `json:"premium,omitempty"` // For Youtube
	Error     string `json:"error,omitempty"`
}

// SaveJSON 保存原始详细报告 (保留旧功能)
func SaveJSON(report models.TestReport, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	filename := fmt.Sprintf("test_result_%s.json", time.Now().Format("20060102_150405"))
	path := filepath.Join(outputDir, filename)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveTagMapJSON 保存为 SubStore 易读的 Map 格式
func SaveTagMapJSON(report models.TestReport, outputPath string) error {
	// 确保父目录存在
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tagMap := make(map[string]NodeTagData)

	for _, result := range report.Results {
		data := NodeTagData{
			UpdateTime: time.Now(),
		}

		// AI Services
		if t, ok := result.Tests["openai"]; ok {
			data.OpenAI = &t
		}
		if t, ok := result.Tests["gemini"]; ok {
			data.Gemini = &t
		}
		if t, ok := result.Tests["claude"]; ok {
			data.Claude = &t
		}

		// Stream Services
		if t, ok := result.StreamTests["netflix"]; ok {
			data.Netflix = &StreamTagData{
				Available: t.Available,
				Region:    t.Region,
				Result:    t.Details, // "Full" or "Originals Only"
				Error:     t.Error,
			}
		}
		if t, ok := result.StreamTests["disney"]; ok {
			data.Disney = &StreamTagData{
				Available: t.Available,
				Region:    t.Region,
				Error:     t.Error,
			}
		}
		if t, ok := result.StreamTests["max"]; ok {
			data.Max = &StreamTagData{
				Available: t.Available,
				Region:    t.Region,
				Error:     t.Error,
			}
		}
		if t, ok := result.StreamTests["youtube"]; ok {
			isPremium := t.Details == "Premium Available"
			data.Youtube = &StreamTagData{
				Available: t.Available,
				Region:    t.Region,
				Premium:   isPremium,
				Error:     t.Error,
			}
		}

		// Key is Node Name
		tagMap[result.NodeName] = data
	}

	jsonData, err := json.MarshalIndent(tagMap, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, jsonData, 0644)
}