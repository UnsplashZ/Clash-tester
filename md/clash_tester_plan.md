# Clash-tester 项目开发文档

## 项目概述

一个轻量级的命令行工具，用于测试Clash/Mihomo订阅中各节点对OpenAI、Gemini、Claude三个AI服务的解锁情况。

### 核心特性
- 跨平台支持（macOS/Windows/Linux）
- 支持主流协议（hysteria2/trojan/vless/anytls）
- 串行测试，10秒超时，失败重试2次
- 支持在线订阅链接和本地YAML配置
- JSON格式结果输出 + 可读性强的控制台展示

---

## 项目结构

```
Clash-tester/
├── cmd/
│   └── main.go                      # 程序入口
├── internal/
│   ├── config/
│   │   └── loader.go                # 配置加载（在线/本地）
│   ├── parser/
│   │   ├── subscription.go          # 订阅解析
│   │   └── yaml.go                  # YAML配置解析
│   ├── proxy/
│   │   ├── dialer.go                # 代理拨号器
│   │   ├── hysteria2.go             # Hysteria2 支持
│   │   ├── trojan.go                # Trojan 支持
│   │   ├── vless.go                 # VLESS 支持
│   │   └── anytls.go                # AnyTLS 支持
│   ├── tester/
│   │   ├── base.go                  # 测试基础框架
│   │   ├── openai.go                # OpenAI 解锁测试
│   │   ├── gemini.go                # Gemini 解锁测试
│   │   ├── claude.go                # Claude 解锁测试
│   │   └── geoip.go                 # IP地理位置检测
│   └── reporter/
│       ├── console.go               # 控制台输出
│       └── json.go                  # JSON报告生成
├── pkg/
│   └── models/
│       └── types.go                 # 数据模型定义
├── configs/
│   └── config.example.yaml          # 配置文件示例
├── result/                          # 测试结果输出目录
├── go.mod
├── go.sum
├── Makefile                         # 编译脚本
└── README.md
```

---

## 数据模型设计

### 核心数据结构

```go
// pkg/models/types.go

package models

import "time"

// ProxyNode 代理节点
type ProxyNode struct {
    Name     string            `yaml:"name"`
    Type     string            `yaml:"type"`     // hysteria2, trojan, vless, ss
    Server   string            `yaml:"server"`
    Port     int               `yaml:"port"`
    Password string            `yaml:"password,omitempty"`
    UUID     string            `yaml:"uuid,omitempty"`
    Params   map[string]interface{} `yaml:",inline"` // 其他参数
}

// ServiceTest 单个服务的测试结果
type ServiceTest struct {
    Service     string `json:"service"`      // OpenAI/Gemini/Claude
    Available   bool   `json:"available"`
    Country     string `json:"country,omitempty"`
    Region      string `json:"region,omitempty"`
    StatusCode  int    `json:"status_code,omitempty"`
    ResponseTime int   `json:"response_time_ms,omitempty"`
    Error       string `json:"error,omitempty"`
    Attempts    int    `json:"attempts"`     // 尝试次数
}

// NodeTestResult 单个节点的完整测试结果
type NodeTestResult struct {
    NodeName    string                  `json:"node_name"`
    NodeType    string                  `json:"node_type"`
    Server      string                  `json:"server"`
    Tests       map[string]ServiceTest  `json:"tests"` // key: openai/gemini/claude
    TotalTime   int                     `json:"total_time_ms"`
}

// TestReport 完整测试报告
type TestReport struct {
    TestTime       time.Time        `json:"test_time"`
    Source         string           `json:"source"`          // 订阅URL或文件路径
    TotalNodes     int              `json:"total_nodes"`
    TestedNodes    int              `json:"tested_nodes"`
    SuccessNodes   int              `json:"success_nodes"`   // 至少一个服务可用
    Results        []NodeTestResult `json:"results"`
    Summary        TestSummary      `json:"summary"`
}

// TestSummary 测试摘要
type TestSummary struct {
    OpenAI  ServiceSummary `json:"openai"`
    Gemini  ServiceSummary `json:"gemini"`
    Claude  ServiceSummary `json:"claude"`
}

// ServiceSummary 单个服务的统计
type ServiceSummary struct {
    Available   int      `json:"available_count"`
    Unavailable int      `json:"unavailable_count"`
    Countries   []string `json:"countries"` // 可用的国家列表
}
```

---

## 核心模块实现

### 1. 配置加载模块

```go
// internal/config/loader.go

package config

import (
    "encoding/base64"
    "io"
    "net/http"
    "os"
    "strings"
    "gopkg.in/yaml.v3"
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
    if decoded, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
        return decoded, nil
    }
    
    return data, nil
}

// loadFromFile 从本地文件加载
func loadFromFile(path string) ([]byte, error) {
    return os.ReadFile(path)
}
```

### 2. 订阅解析模块

```go
// internal/parser/subscription.go

package parser

import (
    "gopkg.in/yaml.v3"
    "Clash-tester/pkg/models"
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
    }
    return supported[protocol]
}
```

### 3. 代理拨号器基础框架

```go
// internal/proxy/dialer.go

package proxy

import (
    "context"
    "net"
    "time"
    "Clash-tester/pkg/models"
)

// Dialer 代理拨号器接口
type Dialer interface {
    Dial(network, addr string) (net.Conn, error)
    DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// NewDialer 根据节点类型创建拨号器
func NewDialer(node models.ProxyNode, timeout time.Duration) (Dialer, error) {
    switch node.Type {
    case "hysteria2":
        return NewHysteria2Dialer(node, timeout)
    case "trojan":
        return NewTrojanDialer(node, timeout)
    case "vless":
        return NewVLESSDialer(node, timeout)
    default:
        return nil, fmt.Errorf("unsupported protocol: %s", node.Type)
    }
}
```

### 4. AI服务测试框架

```go
// internal/tester/base.go

package tester

import (
    "context"
    "net/http"
    "time"
    "Clash-tester/pkg/models"
    "Clash-tester/internal/proxy"
)

const (
    MaxRetries = 2
    TestTimeout = 10 * time.Second
)

// TestService 测试单个服务
func TestService(node models.ProxyNode, serviceName string) models.ServiceTest {
    result := models.ServiceTest{
        Service:  serviceName,
        Attempts: 0,
    }
    
    // 重试逻辑
    for attempt := 0; attempt <= MaxRetries; attempt++ {
        result.Attempts++
        
        start := time.Now()
        err := testWithRetry(node, serviceName, &result)
        result.ResponseTime = int(time.Since(start).Milliseconds())
        
        if err == nil {
            result.Available = true
            return result
        }
        
        result.Error = err.Error()
        
        // 最后一次尝试失败
        if attempt == MaxRetries {
            result.Available = false
        }
    }
    
    return result
}

func testWithRetry(node models.ProxyNode, service string, result *models.ServiceTest) error {
    dialer, err := proxy.NewDialer(node, TestTimeout)
    if err != nil {
        return err
    }
    
    client := &http.Client{
        Timeout: TestTimeout,
        Transport: &http.Transport{
            DialContext: dialer.DialContext,
        },
    }
    
    switch service {
    case "openai":
        return testOpenAI(client, result)
    case "gemini":
        return testGemini(client, result)
    case "claude":
        return testClaude(client, result)
    default:
        return fmt.Errorf("unknown service: %s", service)
    }
}
```

```go
// internal/tester/openai.go

package tester

import (
    "io"
    "net/http"
    "strings"
    "Clash-tester/pkg/models"
)

func testOpenAI(client *http.Client, result *models.ServiceTest) error {
    // 使用Cloudflare trace获取地理位置
    req, _ := http.NewRequest("GET", "https://chat.openai.com/cdn-cgi/trace", nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
    
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    result.StatusCode = resp.StatusCode
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("status code: %d", resp.StatusCode)
    }
    
    // 解析trace信息获取国家
    body, _ := io.ReadAll(resp.Body)
    lines := strings.Split(string(body), "\n")
    for _, line := range lines {
        if strings.HasPrefix(line, "loc=") {
            result.Country = strings.TrimPrefix(line, "loc=")
            break
        }
    }
    
    return nil
}
```

```go
// internal/tester/gemini.go

package tester

import (
    "net/http"
    "Clash-tester/pkg/models"
)

func testGemini(client *http.Client, result *models.ServiceTest) error {
    req, _ := http.NewRequest("GET", "https://gemini.google.com/", nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
    
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    result.StatusCode = resp.StatusCode
    
    // 403/451 通常表示地区限制
    if resp.StatusCode == 403 || resp.StatusCode == 451 {
        return fmt.Errorf("region blocked")
    }
    
    // 通过IP API获取国家信息
    result.Country, _ = getCountryByIP(client)
    
    return nil
}
```

```go
// internal/tester/claude.go

package tester

import (
    "io"
    "net/http"
    "strings"
    "Clash-tester/pkg/models"
)

func testClaude(client *http.Client, result *models.ServiceTest) error {
    req, _ := http.NewRequest("GET", "https://claude.ai/", nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
    
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    result.StatusCode = resp.StatusCode
    
    // 检查是否有地区限制提示
    body, _ := io.ReadAll(resp.Body)
    bodyStr := string(body)
    
    if strings.Contains(bodyStr, "not available") || 
       strings.Contains(bodyStr, "unavailable in your country") {
        return fmt.Errorf("region blocked")
    }
    
    result.Country, _ = getCountryByIP(client)
    
    return nil
}
```

```go
// internal/tester/geoip.go

package tester

import (
    "encoding/json"
    "net/http"
)

type IPAPIResponse struct {
    Country     string `json:"country"`
    CountryCode string `json:"countryCode"`
    Region      string `json:"regionName"`
}

func getCountryByIP(client *http.Client) (string, error) {
    resp, err := client.Get("http://ip-api.com/json/")
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    var result IPAPIResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    
    return result.CountryCode, nil
}
```

### 5. 结果输出模块

```go
// internal/reporter/json.go

package reporter

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"
    "Clash-tester/pkg/models"
)

func SaveJSON(report models.TestReport, outputDir string) error {
    // 确保目录存在
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        return err
    }
    
    // 生成文件名
    filename := fmt.Sprintf("test_result_%s.json", 
        time.Now().Format("20060102_150405"))
    filepath := filepath.Join(outputDir, filename)
    
    // 序列化JSON
    data, err := json.MarshalIndent(report, "", "  ")
    if err != nil {
        return err
    }
    
    // 写入文件
    return os.WriteFile(filepath, data, 0644)
}
```

```go
// internal/reporter/console.go

package reporter

import (
    "fmt"
    "Clash-tester/pkg/models"
)

func PrintConsole(report models.TestReport) {
    fmt.Println("\n" + strings.Repeat("=", 80))
    fmt.Printf("Clash AI Service Tester - Test Report\n")
    fmt.Printf("Test Time: %s\n", report.TestTime.Format("2006-01-02 15:04:05"))
    fmt.Printf("Source: %s\n", report.Source)
    fmt.Println(strings.Repeat("=", 80))
    
    fmt.Printf("\nTotal Nodes: %d | Tested: %d | At least one service available: %d\n\n",
        report.TotalNodes, report.TestedNodes, report.SuccessNodes)
    
    // 打印每个节点的结果
    for i, node := range report.Results {
        fmt.Printf("[%d] %s (%s - %s)\n", i+1, node.NodeName, node.NodeType, node.Server)
        
        printServiceResult("OpenAI", node.Tests["openai"])
        printServiceResult("Gemini", node.Tests["gemini"])
        printServiceResult("Claude", node.Tests["claude"])
        
        fmt.Println()
    }
    
    // 打印摘要
    fmt.Println(strings.Repeat("-", 80))
    fmt.Println("Summary:")
    fmt.Printf("  OpenAI:  ✓ %d | ✗ %d | Countries: %v\n",
        report.Summary.OpenAI.Available, report.Summary.OpenAI.Unavailable,
        report.Summary.OpenAI.Countries)
    fmt.Printf("  Gemini:  ✓ %d | ✗ %d | Countries: %v\n",
        report.Summary.Gemini.Available, report.Summary.Gemini.Unavailable,
        report.Summary.Gemini.Countries)
    fmt.Printf("  Claude:  ✓ %d | ✗ %d | Countries: %v\n",
        report.Summary.Claude.Available, report.Summary.Claude.Unavailable,
        report.Summary.Claude.Countries)
    fmt.Println(strings.Repeat("=", 80))
}

func printServiceResult(name string, test models.ServiceTest) {
    status := "✗"
    if test.Available {
        status = "✓"
    }
    
    info := fmt.Sprintf("  %s %s", status, name)
    if test.Available {
        info += fmt.Sprintf(" [%s] (%dms, %d attempts)", 
            test.Country, test.ResponseTime, test.Attempts)
    } else {
        info += fmt.Sprintf(" [Failed: %s]", test.Error)
    }
    
    fmt.Println(info)
}
```

---

## 主程序实现

```go
// cmd/main.go

package main

import (
    "flag"
    "fmt"
    "log"
    "time"
    
    "Clash-tester/internal/config"
    "Clash-tester/internal/parser"
    "Clash-tester/internal/tester"
    "Clash-tester/internal/reporter"
    "Clash-tester/pkg/models"
)

func main() {
    // 命令行参数
    source := flag.String("source", "", "Subscription URL or local YAML file path")
    output := flag.String("output", "result", "Output directory for results")
    flag.Parse()
    
    if *source == "" {
        log.Fatal("Please provide -source parameter")
    }
    
    fmt.Println("Clash AI Service Tester v1.0")
    fmt.Printf("Loading configuration from: %s\n", *source)
    
    // 1. 加载配置
    data, err := config.Load(config.LoaderConfig{
        Source:  *source,
        Timeout: 30,
    })
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    
    // 2. 解析节点
    nodes, err := parser.Parse(data)
    if err != nil {
        log.Fatalf("Failed to parse config: %v", err)
    }
    
    fmt.Printf("Found %d supported nodes\n\n", len(nodes))
    
    // 3. 测试所有节点
    report := models.TestReport{
        TestTime:    time.Now(),
        Source:      *source,
        TotalNodes:  len(nodes),
        Results:     make([]models.NodeTestResult, 0, len(nodes)),
    }
    
    for i, node := range nodes {
        fmt.Printf("[%d/%d] Testing: %s\n", i+1, len(nodes), node.Name)
        
        result := testNode(node)
        report.Results = append(report.Results, result)
        report.TestedNodes++
        
        if isNodeSuccess(result) {
            report.SuccessNodes++
        }
    }
    
    // 4. 生成摘要
    report.Summary = generateSummary(report.Results)
    
    // 5. 输出结果
    reporter.PrintConsole(report)
    
    if err := reporter.SaveJSON(report, *output); err != nil {
        log.Printf("Failed to save JSON: %v", err)
    } else {
        fmt.Printf("\nResults saved to: %s/\n", *output)
    }
}

func testNode(node models.ProxyNode) models.NodeTestResult {
    result := models.NodeTestResult{
        NodeName: node.Name,
        NodeType: node.Type,
        Server:   node.Server,
        Tests:    make(map[string]models.ServiceTest),
    }
    
    start := time.Now()
    
    // 测试三个服务
    result.Tests["openai"] = tester.TestService(node, "openai")
    result.Tests["gemini"] = tester.TestService(node, "gemini")
    result.Tests["claude"] = tester.TestService(node, "claude")
    
    result.TotalTime = int(time.Since(start).Milliseconds())
    
    return result
}

func isNodeSuccess(result models.NodeTestResult) bool {
    for _, test := range result.Tests {
        if test.Available {
            return true
        }
    }
    return false
}

func generateSummary(results []models.NodeTestResult) models.TestSummary {
    summary := models.TestSummary{
        OpenAI: models.ServiceSummary{Countries: []string{}},
        Gemini: models.ServiceSummary{Countries: []string{}},
        Claude: models.ServiceSummary{Countries: []string{}},
    }
    
    countrySet := make(map[string]map[string]bool)
    countrySet["openai"] = make(map[string]bool)
    countrySet["gemini"] = make(map[string]bool)
    countrySet["claude"] = make(map[string]bool)
    
    for _, result := range results {
        updateServiceSummary(&summary.OpenAI, result.Tests["openai"], countrySet["openai"])
        updateServiceSummary(&summary.Gemini, result.Tests["gemini"], countrySet["gemini"])
        updateServiceSummary(&summary.Claude, result.Tests["claude"], countrySet["claude"])
    }
    
    summary.OpenAI.Countries = mapToSlice(countrySet["openai"])
    summary.Gemini.Countries = mapToSlice(countrySet["gemini"])
    summary.Claude.Countries = mapToSlice(countrySet["claude"])
    
    return summary
}

func updateServiceSummary(s *models.ServiceSummary, test models.ServiceTest, countries map[string]bool) {
    if test.Available {
        s.Available++
        if test.Country != "" {
            countries[test.Country] = true
        }
    } else {
        s.Unavailable++
    }
}

func mapToSlice(m map[string]bool) []string {
    result := make([]string, 0, len(m))
    for k := range m {
        result = append(result, k)
    }
    return result
}
```

---

## 编译与部署

### Makefile

```makefile
BINARY_NAME=clash-tester
VERSION=1.0.0

.PHONY: build
build:
	go build -o bin/$(BINARY_NAME) cmd/main.go

.PHONY: build-all
build-all:
	# macOS
	GOOS=darwin GOARCH=amd64 go build -o bin/$(BINARY_NAME)-darwin-amd64 cmd/main.go
	GOOS=darwin GOARCH=arm64 go build -o bin/$(BINARY_NAME)-darwin-arm64 cmd/main.go
	# Linux
	GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY_NAME)-linux-amd64 cmd/main.go
	GOOS=linux GOARCH=arm64 go build -o bin/$(BINARY_NAME)-linux-arm64 cmd/main.go
	# Windows
	GOOS=windows GOARCH=amd64 go build -o bin/$(BINARY_NAME)-windows-amd64.exe cmd/main.go

.PHONY: clean
clean:
	rm -rf bin/

.PHONY: test
test:
	go test -v ./...
```

### 使用示例

```bash
# 在线订阅测试
./clash-tester -source "https://example.com/sub?token=xxx"

# 本地配置测试
./clash-tester -source "./configs/my-config.yaml"

# 指定输出目录
./clash-tester -source "https://example.com/sub" -output "./my-results"
```

---

## 依赖库说明

```go
// go.mod
module Clash-tester

go 1.21

require (
    gopkg.in/yaml.v3 v3.0.1                    // YAML解析
    golang.org/x/net v0.20.0                   // 网络库
)
```

**协议实现库建议**：
- Hysteria2: 可以集成 `github.com/apernet/hysteria` 或自己实现
- Trojan: 使用 `github.com/Dreamacro/clash` 中的实现
- VLESS: 参考 `github.com/XTLS/xray-core` 的实现

由于这些协议比较复杂，建议：
1. 优先使用已有的开源实现（如Clash或Xray的代码）
2. 或者通过本地启动mihomo核心，使用其HTTP API切换节点并测试

---

## 开发优先级

### Phase 1: MVP（最小可用版本）
1. 配置加载（在线+本地）✓
2. YAML解析 ✓
3. 基础测试框架 ✓
4. 至少支持一种协议（建议先Trojan，相对简单）
5. JSON结果输出 ✓
6. 控制台展示 ✓

### Phase 2: 协议完善
1. 添加VLESS支持
2. 添加Hysteria2支持
3. 完善错误处理和重试机制

### Phase 3: 优化增强
1. 添加进度条显示
2. 支持并发测试选项（可选）
3. 添加配置文件支持
4. 性能优化

---

## 潜在问题与解决方案

### 问题1: 协议实现复杂
**解决方案**: 
- 方案A: 集成已有库（Clash/Xray）
- 方案B: 本地启动mihomo核心，通过API控制
- 方案C: 先实现简单协议（SS/Trojan），复杂协议后续添加

### 问题2: 某些服务的检测可能不准确
**解决方案**:
- 多次测试取结果
- 结合多个检测点（CDN trace + IP API）
- 允许用户自定义检测URL

### 问题3: 跨平台编译依赖问题
**解决方案**:
- 纯Go实现，避免CGO
- 使用 `go build -tags netgo` 静态编译
- 针对不同平台测试验证

---

## 后续扩展方向

1. **Web界面**: 添加简单的Web UI展示结果
2. **定时任务**: 支持定时自动测试
3. **通知功能**: 测试完成后发送邮件/Telegram通知
4. **数据库存储**: 保存历史测试记录，支持趋势分析
5. **节点推荐**: 根据测试结果智能推荐最优节点

---

## 配置文件示例

```yaml
# configs/config.example.yaml

# 订阅源（支持多个）
subscriptions:
  - url: "https://example.com/sub1"
    name: "机场1"
  - url: "https://example.com/sub2"
    name: "机场2"

# 测试配置
test:
  timeout: 10                    # 单次测试超时（秒）
  retries: 2                     # 失败重试次数
  services:                      # 要测试的服务
    - openai
    - gemini
    - claude

# 输出配置
output:
  directory: "result"            # 结果输出目录
  console: true                  # 是否控制台输出
  json: true                     # 是否生成JSON
```

---

## 开发建议

1. **从简单开始**: 先实现一个协议，能跑通整个流程后再扩展
2. **模块化开发**: 每个模块独立测试，便于调试
3. **错误处理**: 网络操作必须有完善的错误处理和超时控制
4. **日志记录**: 添加详细的日志，方便排查问题
5. **测试驱动**: 为核心模块编写单元测试

---

## 快速启动指南

### 步骤1: 初始化项目

```bash
mkdir Clash-tester
cd Clash-tester
go mod init Clash-tester

# 创建目录结构
mkdir -p cmd internal/{config,parser,proxy,tester,reporter} pkg/models configs result
```

### 步骤2: 安装依赖

```bash
go get gopkg.in/yaml.v3
go get golang.org/x/net/proxy
```

### 步骤3: 实现核心模块

按照以下顺序开发：
1. `pkg/models/types.go` - 数据结构定义
2. `internal/config/loader.go` - 配置加载
3. `internal/parser/subscription.go` - 订阅解析
4. `internal/tester/base.go` - 测试框架
5. `internal/tester/openai.go` - OpenAI测试
6. `internal/tester/gemini.go` - Gemini测试
7. `internal/tester/claude.go` - Claude测试
8. `internal/reporter/json.go` - JSON输出
9. `internal/reporter/console.go` - 控制台输出
10. `cmd/main.go` - 主程序

### 步骤4: 代理实现选择

**推荐方案：使用mihomo核心的HTTP API**

原因：
- 协议实现复杂度高（特别是hysteria2和vless）
- mihomo已经完美支持所有协议
- 通过API控制更稳定可靠

实现方式：

```go
// internal/proxy/mihomo.go

package proxy

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "os/exec"
    "time"
)

type MihomoCore struct {
    Port    int
    APIPort int
    cmd     *exec.Cmd
}

// Start 启动mihomo核心
func (m *MihomoCore) Start(configPath string) error {
    m.cmd = exec.Command("mihomo", "-f", configPath, "-d", ".")
    
    if err := m.cmd.Start(); err != nil {
        return err
    }
    
    // 等待核心启动
    time.Sleep(2 * time.Second)
    return nil
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
```

修改后的测试流程：

```go
// cmd/main.go 中的测试逻辑

func testWithMihomo(nodes []models.ProxyNode, configPath string) models.TestReport {
    // 1. 启动mihomo核心
    core := &proxy.MihomoCore{
        Port:    7890,
        APIPort: 9090,
    }
    
    if err := core.Start(configPath); err != nil {
        log.Fatalf("Failed to start mihomo: %v", err)
    }
    defer core.Stop()
    
    report := models.TestReport{
        TestTime: time.Now(),
        Source:   configPath,
        Results:  make([]models.NodeTestResult, 0),
    }
    
    // 2. 遍历测试每个节点
    for i, node := range nodes {
        fmt.Printf("[%d/%d] Testing: %s\n", i+1, len(nodes), node.Name)
        
        // 切换到该节点
        if err := core.SwitchProxy(node.Name); err != nil {
            log.Printf("Failed to switch to %s: %v", node.Name, err)
            continue
        }
        
        // 等待代理切换完成
        time.Sleep(1 * time.Second)
        
        // 使用代理进行测试
        result := testNodeWithProxy(node, core.GetProxyURL())
        report.Results = append(report.Results, result)
    }
    
    return report
}

func testNodeWithProxy(node models.ProxyNode, proxyURL string) models.NodeTestResult {
    result := models.NodeTestResult{
        NodeName: node.Name,
        NodeType: node.Type,
        Server:   node.Server,
        Tests:    make(map[string]models.ServiceTest),
    }
    
    // 创建使用代理的HTTP客户端
    proxyURLParsed, _ := url.Parse(proxyURL)
    client := &http.Client{
        Timeout: 10 * time.Second,
        Transport: &http.Transport{
            Proxy: http.ProxyURL(proxyURLParsed),
        },
    }
    
    // 测试三个服务
    result.Tests["openai"] = testServiceWithClient(client, "openai")
    result.Tests["gemini"] = testServiceWithClient(client, "gemini")
    result.Tests["claude"] = testServiceWithClient(client, "claude")
    
    return result
}
```

### 步骤5: 完善mihomo配置生成

```go
// internal/config/mihomo.go

package config

import (
    "gopkg.in/yaml.v3"
    "os"
    "Clash-tester/pkg/models"
)

// GenerateMihomoConfig 为测试生成mihomo配置
func GenerateMihomoConfig(nodes []models.ProxyNode, outputPath string) error {
    config := map[string]interface{}{
        "port":               7890,
        "socks-port":         7891,
        "allow-lan":          false,
        "mode":               "global",
        "log-level":          "silent",
        "external-controller": "127.0.0.1:9090",
        "proxies":            nodes,
        "proxy-groups": []map[string]interface{}{
            {
                "name":    "GLOBAL",
                "type":    "select",
                "proxies": getNodeNames(nodes),
            },
        },
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
```

---

## 完整的主程序流程（使用mihomo）

```go
// cmd/main.go (完整版)

package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    "time"
    
    "Clash-tester/internal/config"
    "Clash-tester/internal/parser"
    "Clash-tester/internal/proxy"
    "Clash-tester/internal/tester"
    "Clash-tester/internal/reporter"
    "Clash-tester/pkg/models"
)

func main() {
    // 命令行参数
    source := flag.String("source", "", "Subscription URL or local YAML file path")
    output := flag.String("output", "result", "Output directory for results")
    mihomoPath := flag.String("mihomo", "mihomo", "Path to mihomo executable")
    flag.Parse()
    
    if *source == "" {
        log.Fatal("Please provide -source parameter")
    }
    
    printBanner()
    
    // 1. 加载配置
    fmt.Printf("📥 Loading configuration from: %s\n", *source)
    data, err := config.Load(config.LoaderConfig{
        Source:  *source,
        Timeout: 30,
    })
    if err != nil {
        log.Fatalf("❌ Failed to load config: %v", err)
    }
    
    // 2. 解析节点
    fmt.Println("🔍 Parsing subscription...")
    nodes, err := parser.Parse(data)
    if err != nil {
        log.Fatalf("❌ Failed to parse config: %v", err)
    }
    
    fmt.Printf("✅ Found %d supported nodes\n\n", len(nodes))
    
    if len(nodes) == 0 {
        log.Fatal("❌ No supported nodes found")
    }
    
    // 3. 生成mihomo配置
    tempConfig := "temp_mihomo_config.yaml"
    fmt.Println("⚙️  Generating mihomo configuration...")
    if err := config.GenerateMihomoConfig(nodes, tempConfig); err != nil {
        log.Fatalf("❌ Failed to generate mihomo config: %v", err)
    }
    defer os.Remove(tempConfig) // 清理临时配置
    
    // 4. 启动mihomo核心
    fmt.Println("🚀 Starting mihomo core...")
    core := proxy.NewMihomoCore(*mihomoPath, tempConfig)
    if err := core.Start(); err != nil {
        log.Fatalf("❌ Failed to start mihomo: %v", err)
    }
    defer core.Stop()
    
    fmt.Println("✅ Mihomo core started\n")
    fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    
    // 5. 测试所有节点
    report := models.TestReport{
        TestTime:   time.Now(),
        Source:     *source,
        TotalNodes: len(nodes),
        Results:    make([]models.NodeTestResult, 0, len(nodes)),
    }
    
    for i, node := range nodes {
        fmt.Printf("\n[%d/%d] 🔄 Testing: %s\n", i+1, len(nodes), node.Name)
        
        // 切换节点
        if err := core.SwitchProxy(node.Name); err != nil {
            log.Printf("⚠️  Failed to switch to %s: %v", node.Name, err)
            continue
        }
        
        // 等待代理生效
        time.Sleep(1 * time.Second)
        
        // 执行测试
        result := tester.TestNode(node, core.GetProxyURL())
        report.Results = append(report.Results, result)
        report.TestedNodes++
        
        if tester.IsNodeSuccess(result) {
            report.SuccessNodes++
        }
        
        // 显示节点测试结果
        printNodeResult(result)
    }
    
    fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    
    // 6. 生成摘要
    report.Summary = tester.GenerateSummary(report.Results)
    
    // 7. 输出结果
    reporter.PrintConsole(report)
    
    if err := reporter.SaveJSON(report, *output); err != nil {
        log.Printf("⚠️  Failed to save JSON: %v", err)
    } else {
        fmt.Printf("\n💾 Results saved to: %s/\n", *output)
    }
    
    fmt.Println("\n✨ Test completed!")
}

func printBanner() {
    banner := `
╔═══════════════════════════════════════════════════════╗
║                                                       ║
║        Clash AI Service Tester v1.0                  ║
║        Test OpenAI, Gemini, Claude Availability       ║
║                                                       ║
╚═══════════════════════════════════════════════════════╝
`
    fmt.Println(banner)
}

func printNodeResult(result models.NodeTestResult) {
    fmt.Printf("  📊 Results:\n")
    for service, test := range result.Tests {
        status := "❌"
        detail := test.Error
        if test.Available {
            status = "✅"
            detail = fmt.Sprintf("%s (%dms, %d attempts)", 
                test.Country, test.ResponseTime, test.Attempts)
        }
        fmt.Printf("     %s %s: %s\n", status, service, detail)
    }
}
```

---

## 增强的测试模块

```go
// internal/tester/service.go

package tester

import (
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"
    
    "Clash-tester/pkg/models"
)

const (
    MaxRetries  = 2
    TestTimeout = 10 * time.Second
)

// TestNode 测试单个节点的所有服务
func TestNode(node models.ProxyNode, proxyURL string) models.NodeTestResult {
    result := models.NodeTestResult{
        NodeName: node.Name,
        NodeType: node.Type,
        Server:   node.Server,
        Tests:    make(map[string]models.ServiceTest),
    }
    
    start := time.Now()
    
    // 创建HTTP客户端
    client := createProxyClient(proxyURL)
    
    // 测试三个服务
    result.Tests["openai"] = testServiceWithRetry(client, "openai", testOpenAI)
    result.Tests["gemini"] = testServiceWithRetry(client, "gemini", testGemini)
    result.Tests["claude"] = testServiceWithRetry(client, "claude", testClaude)
    
    result.TotalTime = int(time.Since(start).Milliseconds())
    
    return result
}

type testFunc func(*http.Client, *models.ServiceTest) error

func testServiceWithRetry(client *http.Client, serviceName string, fn testFunc) models.ServiceTest {
    result := models.ServiceTest{
        Service:  serviceName,
        Attempts: 0,
    }
    
    for attempt := 0; attempt <= MaxRetries; attempt++ {
        result.Attempts++
        
        start := time.Now()
        err := fn(client, &result)
        result.ResponseTime = int(time.Since(start).Milliseconds())
        
        if err == nil {
            result.Available = true
            return result
        }
        
        result.Error = err.Error()
        
        // 如果是最后一次尝试
        if attempt == MaxRetries {
            result.Available = false
        } else {
            // 重试前等待
            time.Sleep(500 * time.Millisecond)
        }
    }
    
    return result
}

func createProxyClient(proxyURL string) *http.Client {
    proxyURLParsed, _ := url.Parse(proxyURL)
    
    return &http.Client{
        Timeout: TestTimeout,
        Transport: &http.Transport{
            Proxy:               http.ProxyURL(proxyURLParsed),
            MaxIdleConns:        10,
            IdleConnTimeout:     30 * time.Second,
            DisableCompression:  false,
        },
    }
}

func testOpenAI(client *http.Client, result *models.ServiceTest) error {
    // 方法1: 使用 Cloudflare trace
    req, _ := http.NewRequest("GET", "https://chat.openai.com/cdn-cgi/trace", nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
    
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    result.StatusCode = resp.StatusCode
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("status code: %d", resp.StatusCode)
    }
    
    // 解析 trace 信息
    body, _ := io.ReadAll(resp.Body)
    lines := strings.Split(string(body), "\n")
    for _, line := range lines {
        if strings.HasPrefix(line, "loc=") {
            result.Country = strings.TrimPrefix(line, "loc=")
            break
        }
    }
    
    // 方法2: 如果trace失败，尝试访问主页
    if result.Country == "" {
        result.Country, _ = getCountryByIP(client)
    }
    
    return nil
}

func testGemini(client *http.Client, result *models.ServiceTest) error {
    req, _ := http.NewRequest("GET", "https://gemini.google.com/app", nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
    
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    result.StatusCode = resp.StatusCode
    
    // 检查地区限制
    if resp.StatusCode == 403 || resp.StatusCode == 451 {
        return fmt.Errorf("region blocked (status: %d)", resp.StatusCode)
    }
    
    // 检查响应内容
    body, _ := io.ReadAll(resp.Body)
    bodyStr := strings.ToLower(string(body))
    
    if strings.Contains(bodyStr, "not available in your country") ||
       strings.Contains(bodyStr, "not supported in your region") {
        return fmt.Errorf("region not supported")
    }
    
    // 获取国家信息
    result.Country, _ = getCountryByIP(client)
    
    return nil
}

func testClaude(client *http.Client, result *models.ServiceTest) error {
    req, _ := http.NewRequest("GET", "https://claude.ai/", nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
    
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    result.StatusCode = resp.StatusCode
    
    // 检查状态码
    if resp.StatusCode == 403 || resp.StatusCode == 451 {
        return fmt.Errorf("region blocked (status: %d)", resp.StatusCode)
    }
    
    // 检查响应内容
    body, _ := io.ReadAll(resp.Body)
    bodyStr := strings.ToLower(string(body))
    
    if strings.Contains(bodyStr, "not available") ||
       strings.Contains(bodyStr, "unavailable in your country") {
        return fmt.Errorf("region not supported")
    }
    
    // 获取国家信息
    result.Country, _ = getCountryByIP(client)
    
    return nil
}

// getCountryByIP 通过IP API获取国家信息
func getCountryByIP(client *http.Client) (string, error) {
    resp, err := client.Get("http://ip-api.com/json/")
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    var result struct {
        Country     string `json:"country"`
        CountryCode string `json:"countryCode"`
    }
    
    body, _ := io.ReadAll(resp.Body)
    if err := json.Unmarshal(body, &result); err != nil {
        return "", err
    }
    
    return result.CountryCode, nil
}

// IsNodeSuccess 判断节点是否至少有一个服务可用
func IsNodeSuccess(result models.NodeTestResult) bool {
    for _, test := range result.Tests {
        if test.Available {
            return true
        }
    }
    return false
}

// GenerateSummary 生成测试摘要
func GenerateSummary(results []models.NodeTestResult) models.TestSummary {
    summary := models.TestSummary{
        OpenAI: models.ServiceSummary{Countries: []string{}},
        Gemini: models.ServiceSummary{Countries: []string{}},
        Claude: models.ServiceSummary{Countries: []string{}},
    }
    
    countrySet := map[string]map[string]bool{
        "openai": make(map[string]bool),
        "gemini": make(map[string]bool),
        "claude": make(map[string]bool),
    }
    
    for _, result := range results {
        updateServiceSummary(&summary.OpenAI, result.Tests["openai"], countrySet["openai"])
        updateServiceSummary(&summary.Gemini, result.Tests["gemini"], countrySet["gemini"])
        updateServiceSummary(&summary.Claude, result.Tests["claude"], countrySet["claude"])
    }
    
    summary.OpenAI.Countries = mapToSlice(countrySet["openai"])
    summary.Gemini.Countries = mapToSlice(countrySet["gemini"])
    summary.Claude.Countries = mapToSlice(countrySet["claude"])
    
    return summary
}

func updateServiceSummary(s *models.ServiceSummary, test models.ServiceTest, countries map[string]bool) {
    if test.Available {
        s.Available++
        if test.Country != "" {
            countries[test.Country] = true
        }
    } else {
        s.Unavailable++
    }
}

func mapToSlice(m map[string]bool) []string {
    result := make([]string, 0, len(m))
    for k := range m {
        result = append(result, k)
    }
    return result
}
```

---

## README.md

```markdown
# Clash-tester

一个轻量级的命令行工具，用于测试 Clash/Mihomo 订阅中各节点对 OpenAI、Gemini、Claude 三个 AI 服务的解锁情况。

## 特性

- ✅ 支持在线订阅链接和本地 YAML 配置
- ✅ 支持主流协议：Hysteria2、Trojan、VLESS、Shadowsocks
- ✅ 自动检测节点国家/地区
- ✅ 失败自动重试（最多2次）
- ✅ JSON 格式结果输出
- ✅ 美观的控制台展示
- ✅ 跨平台支持（macOS/Windows/Linux）

## 依赖

需要安装 [mihomo](https://github.com/MetaCubeX/mihomo) 核心：

```bash
# macOS (Homebrew)
brew install mihomo

# 或从 GitHub 下载对应平台的二进制文件
# https://github.com/MetaCubeX/mihomo/releases
```

## 安装

### 从源码编译

```bash
git clone https://github.com/yourusername/Clash-tester.git
cd Clash-tester
go build -o clash-tester cmd/main.go
```

### 下载预编译版本

前往 [Releases](https://github.com/yourusername/Clash-tester/releases) 页面下载对应平台的可执行文件。

## 使用方法

### 测试在线订阅

```bash
./clash-tester -source "https://example.com/sub?token=xxx"
```

### 测试本地配置

```bash
./clash-tester -source "./my-config.yaml"
```

### 指定输出目录

```bash
./clash-tester -source "https://example.com/sub" -output "./my-results"
```

### 指定 mihomo 路径

```bash
./clash-tester -source "https://example.com/sub" -mihomo "/usr/local/bin/mihomo"
```

## 输出示例

### 控制台输出

```
╔═══════════════════════════════════════════════════════╗
║                                                       ║
║        Clash AI Service Tester v1.0                  ║
║        Test OpenAI, Gemini, Claude Availability       ║
║                                                       ║
╚═══════════════════════════════════════════════════════╝

📥 Loading configuration from: https://example.com/sub
🔍 Parsing subscription...
✅ Found 50 supported nodes

⚙️  Generating mihomo configuration...
🚀 Starting mihomo core...
✅ Mihomo core started

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[1/50] 🔄 Testing: 🇺🇸 US Node 1
  📊 Results:
     ✅ openai: US (234ms, 1 attempts)
     ✅ gemini: US (456ms, 1 attempts)
     ❌ claude: Service unavailable

...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Summary:
  OpenAI:  ✓ 45 | ✗ 5 | Countries: [US, JP, SG, UK]
  Gemini:  ✓ 38 | ✗ 12 | Countries: [US, JP, SG]
  Claude:  ✓ 42 | ✗ 8 | Countries: [US, UK, CA]

💾 Results saved to: result/
✨ Test completed!
```

### JSON 输出

结果保存在 `result/test_result_YYYYMMDD_HHMMSS.json`:

```json
{
  "test_time": "2025-01-19T10:30:00Z",
  "source": "https://example.com/sub",
  "total_nodes": 50,
  "tested_nodes": 50,
  "success_nodes": 48,
  "results": [
    {
      "node_name": "🇺🇸 US Node 1",
      "node_type": "vless",
      "server": "us1.example.com",
      "tests": {
        "openai": {
          "service": "openai",
          "available": true,
          "country": "US",
          "status_code": 200,
          "response_time_ms": 234,
          "attempts": 1
        },
        ...
      },
      "total_time_ms": 1520
    }
  ],
  "summary": {
    "openai": {
      "available_count": 45,
      "unavailable_count": 5,
      "countries": ["US", "JP", "SG", "UK"]
    },
    ...
  }
}
```

## 开发路线图

- [x] 基础框架
- [x] 支持在线订阅和本地配置
- [x] OpenAI/Gemini/Claude 测试
- [x] JSON 和控制台输出
- [ ] 支持更多协议
- [ ] 添加进度条
- [ ] 配置文件支持
- [ ] Web UI
- [ ] Docker 支持

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！
```

---

## 总结

这个文档提供了完整的实现方案，核心要点：

1. **使用 mihomo 核心** - 避免自己实现复杂的代理协议
2. **串行测试** - 逐个节点测试，每个节点10秒超时
3. **重试机制** - 失败重试2次
4. **跨平台** - Go 编译为单文件，支持三大平台
5. **清晰的输出** - JSON 文件 + 美观的控制台展示

开发时建议从简单的部分开始，先让整个流程跑通，再逐步完善。