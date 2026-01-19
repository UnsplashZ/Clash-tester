package tester

import (
	"Clash-tester/pkg/models"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// TestStreamingService 测试流媒体服务
func TestStreamingService(client *http.Client, serviceName string) models.StreamTest {
	result := models.StreamTest{
		Service: serviceName,
	}

	start := time.Now()
	var err error

	// 强制设置 User-Agent，防止被 WAF 拦截 (在具体函数中设置)
	// client.Transport.(*http.Transport).DisableKeepAlives = true // 可能会影响复用，视情况而定

	switch serviceName {
	case "netflix":
		err = testNetflix(client, &result)
	case "disney":
		err = testDisney(client, &result)
	case "youtube":
		err = testYoutube(client, &result)
	case "max":
		err = testMax(client, &result)
	default:
		err = fmt.Errorf("unknown service: %s", serviceName)
	}

	result.ResponseTime = int(time.Since(start).Milliseconds())

	if err == nil {
		result.Available = true
	} else {
		result.Available = false
		result.Error = err.Error()
	}

	return result
}

// testNetflix Netflix 双 ID 检测法
func testNetflix(client *http.Client, result *models.StreamTest) error {
	// 1. Check Full Unlock (Breaking Bad - 非自制剧)
	// 如果能看非自制剧，说明是完整解锁
	if checkNetflixURL(client, "https://www.netflix.com/title/70143836", "Breaking Bad", result) {
		result.Details = "Full"
		// 尝试提取地区
		if result.Region == "" {
			result.Region, _ = getCountryByIP(client) // Fallback
		}
		return nil
	}

	// 2. Check Originals (Squid Game - 自制剧)
	// 如果只能看自制剧，说明是部分解锁
	if checkNetflixURL(client, "https://www.netflix.com/title/81243996", "Squid Game", result) {
		result.Details = "Originals Only"
		if result.Region == "" {
			result.Region, _ = getCountryByIP(client) // Fallback
		}
		return nil
	}

	return fmt.Errorf("blocked")
}

func checkNetflixURL(client *http.Client, url, keyword string, result *models.StreamTest) bool {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}

	// 检查重定向 (Response URL)
	finalURL := resp.Request.URL.String()
	if strings.Contains(finalURL, "/browse/genre/") || strings.Contains(finalURL, "NotAvailable") {
		return false
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// 检查页面内容是否包含关键词 (确认是视频页面)
	// Netflix 页面结构经常变，这里用更通用的 "watch-video" 或者是 keyword
	// 另外检查 "current_country" 提取地区
	
	// 提取地区
	re := regexp.MustCompile(`"current_country":"(.*?)"`)
	matches := re.FindStringSubmatch(bodyStr)
	if len(matches) > 1 {
		result.Region = matches[1]
	}

	return strings.Contains(bodyStr, keyword) || strings.Contains(bodyStr, "watch-video")
}


func testDisney(client *http.Client, result *models.StreamTest) error {
	// 临时修改 Client 以拦截重定向
	originalCheck := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() { client.CheckRedirect = originalCheck }()

	req, _ := http.NewRequest("GET", "https://www.disneyplus.com/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 分析 Location
	if resp.StatusCode == 302 || resp.StatusCode == 301 {
		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "/preview") || strings.Contains(loc, "/unavailable") {
			return fmt.Errorf("redirected to preview/unavailable")
		}
		// 跳转到 login 或 home 视为成功
		result.Region, _ = getCountryByIP(client) // Disney+ 很难从 URL 直接看地区，用 IP 辅助
		return nil
	}

	// 如果直接 200 (极少见，通常都会重定向到本地化路径)
	if resp.StatusCode == 200 {
		result.Region, _ = getCountryByIP(client)
		return nil
	}
	
	if resp.StatusCode == 403 {
		return fmt.Errorf("blocked (403)")
	}

	return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

func testYoutube(client *http.Client, result *models.StreamTest) error {
	req, _ := http.NewRequest("GET", "https://www.youtube.com/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// 设置 Cookie 可能会更准确，但这里先不需要

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status: %d", resp.StatusCode)
	}
	
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	
	// 提取地区
	// "countryCode":"US"
	re := regexp.MustCompile(`"countryCode":"(.*?)"`)
	matches := re.FindStringSubmatch(bodyStr)
	if len(matches) > 1 {
		result.Region = matches[1]
	} else {
		// 备用匹配 "ISO_COUNTRY_CODE":"US"
		re2 := regexp.MustCompile(`"ISO_COUNTRY_CODE":"(.*?)"`)
		matches2 := re2.FindStringSubmatch(bodyStr)
		if len(matches2) > 1 {
			result.Region = matches2[1]
		}
	}
	
	if result.Region == "" {
		result.Region, _ = getCountryByIP(client)
	}

	// Premium 检测 (简单版)
	if strings.Contains(bodyStr, "Premium") {
		result.Details = "Premium Available"
	}

	return nil
}

func testMax(client *http.Client, result *models.StreamTest) error {
	req, _ := http.NewRequest("GET", "https://www.max.com/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 405 {
		return fmt.Errorf("blocked (%d)", resp.StatusCode)
	}

	if resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		
		if strings.Contains(bodyStr, "Not Available in your region") || strings.Contains(bodyStr, "GeoBlock") {
			return fmt.Errorf("geo blocked")
		}
		
		result.Region, _ = getCountryByIP(client)
		return nil
	}
	
	return fmt.Errorf("status: %d", resp.StatusCode)
}