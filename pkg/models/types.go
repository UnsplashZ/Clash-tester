package models

import "time"

// ProxyNode 代理节点
type ProxyNode struct {
	Name     string                 `yaml:"name"`
	Type     string                 `yaml:"type"` // hysteria2, trojan, vless, ss
	Server   string                 `yaml:"server"`
	Port     int                    `yaml:"port"`
	Password string                 `yaml:"password,omitempty"`
	UUID     string                 `yaml:"uuid,omitempty"`
	Cipher   string                 `yaml:"cipher,omitempty"`
	Params   map[string]interface{} `yaml:",inline"` // 其他参数
}

// ServiceTest 单个服务的测试结果 (AI Services)
type ServiceTest struct {
	Service      string `json:"service"` // OpenAI/Gemini/Claude
	Available    bool   `json:"available"`
	Country      string `json:"country,omitempty"`
	Region       string `json:"region,omitempty"`
	StatusCode   int    `json:"status_code,omitempty"`
	ResponseTime int    `json:"response_time_ms,omitempty"`
	Error        string `json:"error,omitempty"`
	Attempts     int    `json:"attempts"` // 尝试次数
}

// StreamTest 单个流媒体服务的测试结果
type StreamTest struct {
	Service      string `json:"service"` // Netflix, Disney+, etc.
	Available    bool   `json:"available"`
	Region       string `json:"region,omitempty"` // US, SG, HK, or "Originals Only"
	Details      string `json:"details,omitempty"`
	ResponseTime int    `json:"response_time_ms,omitempty"`
	Error        string `json:"error,omitempty"`
}

// NodeTestResult 单个节点的完整测试结果
type NodeTestResult struct {
	NodeName    string                 `json:"node_name"`
	NodeType    string                 `json:"node_type"`
	Server      string                 `json:"server"`
	Tests       map[string]ServiceTest `json:"tests"`        // key: openai/gemini/claude
	StreamTests map[string]StreamTest  `json:"stream_tests"` // key: netflix/disney/youtube
	TotalTime   int                    `json:"total_time_ms"`
}

// TestReport 完整测试报告
type TestReport struct {
	TestTime     time.Time        `json:"test_time"`
	Source       string           `json:"source"` // 订阅URL或文件路径
	TotalNodes   int              `json:"total_nodes"`
	TestedNodes  int              `json:"tested_nodes"`
	SuccessNodes int              `json:"success_nodes"` // 至少一个服务可用
	Results      []NodeTestResult `json:"results"`
	Summary      TestSummary      `json:"summary"`
}

// TestSummary 测试摘要
type TestSummary struct {
	OpenAI    ServiceSummary            `json:"openai"`
	Gemini    ServiceSummary            `json:"gemini"`
	Claude    ServiceSummary            `json:"claude"`
	Streaming map[string]ServiceSummary `json:"streaming"` // Netflix, Disney, etc.
}

// ServiceSummary 单个服务的统计
type ServiceSummary struct {
	Available   int      `json:"available_count"`
	Unavailable int      `json:"unavailable_count"`
	Countries   []string `json:"countries"` // 可用的国家列表
}