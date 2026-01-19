package tester

import (
	"Clash-tester/pkg/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	MaxRetries  = 2
	TestTimeout = 10 * time.Second
)

// TestNode 测试单个节点的所有服务
func TestNode(node models.ProxyNode, proxyURL string) models.NodeTestResult {
	result := models.NodeTestResult{
		NodeName:    node.Name,
		NodeType:    node.Type,
		Server:      node.Server,
		Tests:       make(map[string]models.ServiceTest),
		StreamTests: make(map[string]models.StreamTest),
	}

	start := time.Now()

	// 创建HTTP客户端
	client := createProxyClient(proxyURL)

	// 测试 AI 服务
	result.Tests["openai"] = testServiceWithRetry(client, "openai", testOpenAI)
	result.Tests["gemini"] = testServiceWithRetry(client, "gemini", testGemini)
	result.Tests["claude"] = testServiceWithRetry(client, "claude", testClaude)

	// 测试流媒体服务
	result.StreamTests["netflix"] = TestStreamingService(client, "netflix")
	result.StreamTests["disney"] = TestStreamingService(client, "disney")
	result.StreamTests["youtube"] = TestStreamingService(client, "youtube")
	result.StreamTests["max"] = TestStreamingService(client, "max")

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
			Proxy:              http.ProxyURL(proxyURLParsed),
			MaxIdleConns:       10,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
		},
	}
}

func testOpenAI(client *http.Client, result *models.ServiceTest) error {
	req, _ := http.NewRequest("GET", "https://chatgpt.com/cdn-cgi/trace", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode == 403 {
		return fmt.Errorf("Cloudflare blocked (403)")
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if strings.Contains(bodyStr, "loc=") {
		lines := strings.Split(bodyStr, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "loc=") {
				result.Country = strings.TrimPrefix(line, "loc=")
				return nil
			}
		}
	}

	return fmt.Errorf("trace info not found")
}

func testGemini(client *http.Client, result *models.ServiceTest) error {
	originalCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() { client.CheckRedirect = originalCheckRedirect }()

	req, _ := http.NewRequest("GET", "https://gemini.google.com/app", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode == 200 {
		result.Country, _ = getCountryByIP(client)
		return nil
	} else if resp.StatusCode == 302 || resp.StatusCode == 301 {
		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "accounts.google.com") {
			result.Country, _ = getCountryByIP(client)
			return nil
		}
		return fmt.Errorf("redirected to unsupported page")
	} else if resp.StatusCode == 403 || resp.StatusCode == 451 {
		return fmt.Errorf("region blocked (%d)", resp.StatusCode)
	}

	return fmt.Errorf("unknown status: %d", resp.StatusCode)
}

func testClaude(client *http.Client, result *models.ServiceTest) error {
	req, _ := http.NewRequest("GET", "https://claude.ai/login", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode == 403 {
		return fmt.Errorf("IP blocked (403 Forbidden)")
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := strings.ToLower(string(body))

	failKeywords := []string{
		"app unavailable",
		"unavailable in your country",
		"coming soon",
	}

	for _, kw := range failKeywords {
		if strings.Contains(bodyStr, kw) {
			return fmt.Errorf("region not supported")
		}
	}

	result.Country, _ = getCountryByIP(client)
	return nil
}

func getCountryByIP(client *http.Client) (string, error) {
	resp, err := client.Get("http://ip-api.com/json/?fields=countryCode")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
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
	// 也检查流媒体？或者AI优先？这里暂且只看AI，或者你可以改为 OR 逻辑
	// for _, test := range result.StreamTests {
	// 	if test.Available {
	// 		return true
	// 	}
	// }
	return false
}

// GenerateSummary 生成测试摘要
func GenerateSummary(results []models.NodeTestResult) models.TestSummary {
	summary := models.TestSummary{
		OpenAI:    models.ServiceSummary{Countries: []string{}},
		Gemini:    models.ServiceSummary{Countries: []string{}},
		Claude:    models.ServiceSummary{Countries: []string{}},
		Streaming: make(map[string]models.ServiceSummary),
	}

	// Initialize streaming summary map
	streamServices := []string{"netflix", "disney", "youtube", "max"}
	for _, s := range streamServices {
		summary.Streaming[s] = models.ServiceSummary{Countries: []string{}}
	}

	countrySet := map[string]map[string]bool{
		"openai": make(map[string]bool),
		"gemini": make(map[string]bool),
		"claude": make(map[string]bool),
	}
	
	streamCountrySet := make(map[string]map[string]bool)
	for _, s := range streamServices {
		streamCountrySet[s] = make(map[string]bool)
	}

	for _, result := range results {
		updateServiceSummary(&summary.OpenAI, result.Tests["openai"], countrySet["openai"])
		updateServiceSummary(&summary.Gemini, result.Tests["gemini"], countrySet["gemini"])
		updateServiceSummary(&summary.Claude, result.Tests["claude"], countrySet["claude"])
		
		for _, s := range streamServices {
			if test, ok := result.StreamTests[s]; ok {
				// We need a ServiceTest-like struct or adapter since StreamTest is different
				// But ServiceSummary logic is simple.
				sSummary := summary.Streaming[s]
				if test.Available {
					sSummary.Available++
					if test.Region != "" {
						streamCountrySet[s][test.Region] = true
					}
				} else {
					sSummary.Unavailable++
				}
				summary.Streaming[s] = sSummary
			}
		}
	}

	summary.OpenAI.Countries = mapToSlice(countrySet["openai"])
	summary.Gemini.Countries = mapToSlice(countrySet["gemini"])
	summary.Claude.Countries = mapToSlice(countrySet["claude"])
	
	for _, s := range streamServices {
		sSummary := summary.Streaming[s]
		sSummary.Countries = mapToSlice(streamCountrySet[s])
		summary.Streaming[s] = sSummary
	}

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
