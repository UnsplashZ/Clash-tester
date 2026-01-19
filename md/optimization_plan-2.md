# 流媒体检测模块设计文档 (Streaming Media Detection)

本文档详细规划了 `Clash-tester` 项目中流媒体检测模块 (`internal/tester/streaming.go`) 的实现细节。该模块旨在通过 HTTP 请求准确判断节点对主流流媒体服务的解锁情况及解锁区域。

## 1. 核心设计原则

1.  **准确性优先**：放弃简单的 HTTP 状态码判断，采用“内容特征 + 重定向分析 + 关键 API”的多重验证机制。
2.  **分级检测**：针对 Netflix 等服务，区分“完整解锁 (Full)”与“仅自制剧 (Originals Only)”。
3.  **环境伪装**：所有请求必须携带主流浏览器 `User-Agent`，防止被 WAF 直接拦截。
4.  **重定向控制**：流媒体检测高度依赖重定向路径分析，HTTP Client 需根据服务特性控制是否自动跟随跳转。

---

## 2. 数据模型设计

更新 `pkg/models/types.go`，增加流媒体测试结果结构体：

```go
type StreamTestResult struct {
    Service      string `json:"service"`       // Netflix, Disney+, YouTube, Max
    Available    bool   `json:"available"`     // 是否可用
    Region       string `json:"region"`        // 解锁地区 (如 US, HK, SG)
    UnlockType   string `json:"unlock_type"`   // 针对 Netflix: "Full", "Originals", "None"
    ResponseTime int    `json:"response_time"` // 响应耗时 (ms)
    Error        string `json:"error,omitempty"`
}

```

---

## 3. 各服务检测逻辑详解

### 3.1 Netflix (奈飞)

Netflix 的封锁机制最为复杂，分为“完全封锁”、“仅自制剧”和“完整解锁”三个等级。我们采用 **双 ID 检测法**。

* **检测逻辑**：
1. **第一步：检测非自制剧 (高标准)**
* **Target**: `https://www.netflix.com/title/70143836` (绝命毒师 - 全球有版权但非独占)
* **判断**:
* HTTP 200 + 页面包含 "Breaking Bad" -> **Full (完整解锁)** -> 结束。
* HTTP 404 / 302 -> 进入第二步。




2. **第二步：检测自制剧 (低标准)**
* **Target**: `https://www.netflix.com/title/81243996` (鱿鱼游戏 - Netflix Original)
* **判断**:
* HTTP 200 -> **Originals Only (仅自制剧)** -> 结束。
* HTTP 403 / 404 / 跳转至错误页 -> **None (解锁失败)**。






* **地区获取**:
* 在成功响应的 HTML Body 中正则匹配 `"current_country":"(.*?)"` 或从重定向 URL 中提取地区代码（如 `/us/`）。



### 3.2 Disney+ (迪士尼+)

Disney+ 对 IP 变动非常敏感，通常通过重定向将用户导向 `/preview` (预览页) 或 `/unavailable` 页面。

* **Target**: `https://www.disneyplus.com/`
* **关键配置**: 禁止自动重定向 (`CheckRedirect` 返回 `http.ErrUseLastResponse`)，手动分析 `Location` 头。
* **检测逻辑**:
1. 发起 GET 请求。
2. **分析状态码与 Location**:
* **HTTP 200**: 成功。
* **HTTP 302**: 检查 `Location` 目标：
* 包含 `/login`, `/home`, `/en-us` 等 -> **Available (成功)**。
* 包含 `/preview`, `/unavailable` -> **Unavailable (失败)**。




3. **失败判定**: 若跳转至 `preview` 页面，视为该地区未开通或 IP 被封锁。



### 3.3 YouTube (油管)

YouTube 几乎不封锁 IP 访问视频，但会根据 IP 归属地推送不同内容（Premium 价格、地区限定视频）。

* **Target**: `https://www.youtube.com/`
* **检测逻辑**:
1. 发起 GET 请求。
2. **地区判定**: 在返回的 HTML 源码中正则匹配以下字段：
* `"countryCode":"(.*?)"`
* `"ISO_COUNTRY_CODE":"(.*?)"`


3. **Premium 判定 (可选)**: 检查源码是否包含 `Premium` 相关购买按钮，或是否重定向至 `premium_unavailable`。



### 3.4 Max (原 HBO Max)

Max 对数据中心 IP 封锁极其严厉，通常直接返回 403/405 或断开连接。

* **Target**: `https://www.max.com/`
* **检测逻辑**:
1. 发起 GET 请求。
2. **判断**:
* HTTP 403 / 405 -> **Unavailable (IP 被封锁)**。
* HTTP 200 -> 检查 Body 内容：
* 若包含 "Not Available in your region" 或 "GeoBlock" -> **Unavailable (地区不支持)**。
* 否则 -> **Available (成功)**。







---

## 4. 代码实现建议 (`internal/tester/streaming.go`)

以下是核心检测函数的伪代码结构：

```go
package tester

import (
    "net/http"
    "regexp"
    "strings"
    "time"
    "Clash-tester/pkg/models"
)

// 通用流媒体测试入口
func TestStreaming(client *http.Client, service string) models.StreamTestResult {
    // 强制设置 User-Agent，防止被 WAF 拦截
    client.Transport.(*http.Transport).DisableKeepAlives = true
    
    switch service {
    case "netflix":
        return testNetflix(client)
    case "disney":
        return testDisney(client)
    case "youtube":
        return testYouTube(client)
    case "max":
        return testMax(client)
    default:
        return models.StreamTestResult{Error: "Unknown service"}
    }
}

// Netflix 双 ID 检测实现
func testNetflix(client *http.Client) models.StreamTestResult {
    result := models.StreamTestResult{Service: "Netflix"}
    
    // 1. Check Full Unlock (Breaking Bad)
    if checkURL(client, "[https://www.netflix.com/title/70143836](https://www.netflix.com/title/70143836)", "Breaking Bad") {
        result.Available = true
        result.UnlockType = "Full"
        result.Region = extractNetflixRegion(client) // 需单独实现
        return result
    }

    // 2. Check Originals (Squid Game)
    if checkURL(client, "[https://www.netflix.com/title/81243996](https://www.netflix.com/title/81243996)", "Squid Game") {
        result.Available = true
        result.UnlockType = "Originals Only"
        result.Region = extractNetflixRegion(client)
        return result
    }

    result.Available = false
    result.UnlockType = "None"
    return result
}

// Disney+ 重定向分析实现
func testDisney(client *http.Client) models.StreamTestResult {
    // 临时修改 Client 以拦截重定向
    originalCheck := client.CheckRedirect
    client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
        return http.ErrUseLastResponse
    }
    defer func() { client.CheckRedirect = originalCheck }()

    req, _ := http.NewRequest("GET", "[https://www.disneyplus.com/](https://www.disneyplus.com/)", nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 ...") // 务必使用真实 UA

    resp, err := client.Do(req)
    // ... 错误处理 ...

    // 分析 Location
    if resp.StatusCode == 302 || resp.StatusCode == 301 {
        loc := resp.Header.Get("Location")
        if strings.Contains(loc, "/preview") || strings.Contains(loc, "/unavailable") {
             return models.StreamTestResult{Service: "Disney+", Available: false}
        }
        // 跳转到 login 或 home 视为成功
        return models.StreamTestResult{Service: "Disney+", Available: true}
    }
    
    // 如果直接 200
    if resp.StatusCode == 200 {
         return models.StreamTestResult{Service: "Disney+", Available: true}
    }

    return models.StreamTestResult{Service: "Disney+", Available: false}
}

```

## 5. 注意事项与优化

1. **User-Agent 池**: 建议维护一个简单的 UA 列表随机选取，避免特征单一。
2. **超时控制**: 流媒体页面通常较大，建议设置较短的 ReadTimeout (如 5s)，因为我们只需要 Header 或 HTML 前几 KB 的内容即可判断，无需下载完整页面。
3. **并发限制**: 批量测试流媒体时，注意控制并发数，以免被目标网站的风控系统通过 IP 指纹关联封锁。

---

## 6. API 响应示例

当通过 API 模式调用时，返回的 JSON 结构应包含上述详细信息：

```json
{
  "node_name": "HK Node 1",
  "results": {
    "netflix": {
      "available": true,
      "region": "HK",
      "unlock_type": "Full",
      "response_time": 450
    },
    "disney": {
      "available": true,
      "region": "HK",
      "response_time": 320
    },
    "youtube": {
      "available": true,
      "region": "HK",
      "response_time": 120
    }
  }
}