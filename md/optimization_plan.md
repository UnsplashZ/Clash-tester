# Clash-tester 项目优化建议文档

本文档基于主流检测脚本（如 Streaming-Check, Mihomo Party 内置脚本）的逻辑，对 `Clash-tester` 项目的检测地址（URL）和判定逻辑进行优化建议，以提高检测准确率。

## 1. 核心检测地址变更汇总

| 服务 | 原计划 URL | **优化后建议 URL** | 判定核心逻辑 |
| :--- | :--- | :--- | :--- |
| **OpenAI** | `https://chat.openai.com/cdn-cgi/trace` | `https://chatgpt.com/cdn-cgi/trace` | 解析 `loc=XX` 字段，无需额外 IP API |
| **Gemini** | `https://gemini.google.com/` | `https://gemini.google.com/app` | 允许重定向到登录页(302)，拒绝重定向到不支持页 |
| **Claude** | `https://claude.ai/` | `https://claude.ai/login` | 状态码 200 且 Body 无 "unavailable" 关键词 |
| **GeoIP** | `http://ip-api.com/json/` | `http://ip-api.com/json/?fields=countryCode` | 仅在 Gemini/Claude 解锁成功后用于补充地区信息 |

---

## 2. 模块代码优化建议

### 2.1 OpenAI 测试模块 (`internal/tester/openai.go`)

**主要变更**：
1. 更新域名为 `chatgpt.com`。
2. 必须设置 `User-Agent` 以通过 Cloudflare 基础拦截。
3. 直接信任 OpenAI 返回的 `loc` 字段，不再进行二次 IP 查询。

```go
package tester

import (
    "fmt"
    "io"
    "net/http"
    "strings"
    "Clash-tester/pkg/models"
)

func testOpenAI(client *http.Client, result *models.ServiceTest) error {
    // 1. 使用 chatgpt.com 的 trace 接口
    req, _ := http.NewRequest("GET", "[https://chatgpt.com/cdn-cgi/trace](https://chatgpt.com/cdn-cgi/trace)", nil)
    // 关键：伪装 UA，否则极大概率返回 403
    req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
    
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    result.StatusCode = resp.StatusCode
    
    // 2. Cloudflare 拦截判定
    if resp.StatusCode == 403 {
        return fmt.Errorf("Cloudflare blocked (403)")
    }
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
    
    // 3. 解析 Body 获取地区
    body, _ := io.ReadAll(resp.Body)
    bodyStr := string(body)
    
    if strings.Contains(bodyStr, "loc=") {
        lines := strings.Split(bodyStr, "\n")
        for _, line := range lines {
            if strings.HasPrefix(line, "loc=") {
                result.Country = strings.TrimPrefix(line, "loc=")
                return nil // 成功，直接返回
            }
        }
    }
    
    return fmt.Errorf("trace info not found")
}

```

### 2.2 Gemini 测试模块 (`internal/tester/gemini.go`)

**主要变更**：

1. URL 改为 `/app` 后缀，触发应用逻辑。
2. **禁止自动重定向** (`CheckRedirect`)，手动分析 302 跳转目标。
3. 跳转到 `accounts.google.com` 视为成功（需要登录），跳转到 `support.google.com` 或 `unavailable` 视为失败。

```go
package tester

import (
    "fmt"
    "net/http"
    "strings"
    "Clash-tester/pkg/models"
)

func testGemini(client *http.Client, result *models.ServiceTest) error {
    // 关键：修改 Client 行为，禁止自动跟随重定向
    originalCheckRedirect := client.CheckRedirect
    client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
        return http.ErrUseLastResponse // 遇到重定向即停止，返回 302 响应
    }
    // 函数结束时恢复（虽然在这里 client 是临时的，但保持好习惯）
    defer func() { client.CheckRedirect = originalCheckRedirect }()

    req, _ := http.NewRequest("GET", "[https://gemini.google.com/app](https://gemini.google.com/app)", nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
    
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    result.StatusCode = resp.StatusCode

    // 逻辑判定
    if resp.StatusCode == 200 {
        // 极少情况直接进入，需补充 IP 归属地查询
        result.Country, _ = getCountryByIP(client)
        return nil
    } else if resp.StatusCode == 302 || resp.StatusCode == 301 {
        loc := resp.Header.Get("Location")
        // 跳转到登录页 -> 说明 IP 干净且地区支持
        if strings.Contains(loc, "accounts.google.com") {
            result.Country, _ = getCountryByIP(client)
            return nil
        }
        // 跳转到支持页或其他错误页 -> 失败
        return fmt.Errorf("redirected to unsupported page")
    } else if resp.StatusCode == 403 || resp.StatusCode == 451 {
        return fmt.Errorf("region blocked (%d)", resp.StatusCode)
    }

    return fmt.Errorf("unknown status: %d", resp.StatusCode)
}

```

### 2.3 Claude 测试模块 (`internal/tester/claude.go`)

**主要变更**：

1. URL 改为 `/login`，这是目前检测 Claude 最准确的端点。
2. 增加对 Body 内容的关键词检测（防止 200 状态码的“不可用”页面）。

```go
package tester

import (
    "fmt"
    "io"
    "net/http"
    "strings"
    "Clash-tester/pkg/models"
)

func testClaude(client *http.Client, result *models.ServiceTest) error {
    req, _ := http.NewRequest("GET", "[https://claude.ai/login](https://claude.ai/login)", nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
    
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    result.StatusCode = resp.StatusCode
    
    // 1. 严格的状态码检查
    if resp.StatusCode == 403 {
        // Claude 封锁 IDC IP 通常返回 403
        return fmt.Errorf("IP blocked (403 Forbidden)")
    }
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("status code: %d", resp.StatusCode)
    }
    
    // 2. 内容检查 (防误判)
    body, _ := io.ReadAll(resp.Body)
    bodyStr := strings.ToLower(string(body))
    
    // 检查页面是否包含拒绝访问的关键词
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
    
    // 3. 成功，补充地区信息
    result.Country, _ = getCountryByIP(client)
    return nil
}

```

### 2.4 通用 IP 辅助模块 (`internal/tester/geoip.go`)

**主要变更**：

1. 简化 API 调用，只获取 `countryCode` 以减少数据量。
2. 仅作为 Gemini 和 Claude 的后备方案使用。

```go
package tester

import (
    "encoding/json"
    "net/http"
)

// 简单的响应结构
type IPAPIResponse struct {
    CountryCode string `json:"countryCode"`
}

func getCountryByIP(client *http.Client) (string, error) {
    // 加上 fields 参数减少响应体积
    resp, err := client.Get("[http://ip-api.com/json/?fields=countryCode](http://ip-api.com/json/?fields=countryCode)")
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

## 3. 总结

| 模块 | 依赖逻辑 | 备注 |
| --- | --- | --- |
| **OpenAI** | 自身 Trace | 最准确，无需外部 API |
| **Gemini** | 重定向分析 + IP API | 必须处理 HTTP 302 |
| **Claude** | 状态码 + 关键词 + IP API | 必须严格检查 403 |
