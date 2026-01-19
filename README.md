# Clash-tester

一个功能强大的 Clash 节点检测工具，支持 CLI 和 Server 模式。可用于批量测试节点对 AI 服务（OpenAI, Gemini, Claude）及流媒体服务（Netflix, Disney+, YouTube, HBO Max）的解锁情况。支持作为 API 服务集成到 SubStore 等工具中。

## ✨ 特性

- **多模式运行**：
  - **CLI 模式**：本地批量测试订阅链接或配置文件，生成详细报告。
  - **Server 模式**：提供 HTTP API，支持外部脚本动态调用测试。
- **全面的解锁检测**：
  - **AI 服务**：OpenAI (ChatGPT), Google Gemini, Anthropic Claude。
  - **流媒体**：Netflix (双重检测: 自制剧/非自制剧), Disney+, YouTube (Premium/地区), HBO Max。
- **高准确性**：基于页面内容特征、重定向分析及 API 响应的多重判定机制，非简单的状态码检测。
- **高性能**：
  - 基于 Mihomo (Clash Meta) 核心。
  - 支持多 Worker 并发测试。
  - 自动维护 Worker 资源池。
- **易于集成**：提供 Docker 镜像，方便部署；API 设计友好，适配 SubStore 脚本。

## 🚀 快速开始 (CLI 模式)

### 1. 准备环境
确保目录下有 `mihomo` 核心文件 (Windows: `mihomo.exe`, Linux/Mac: `mihomo`)。

### 2. 运行测试
```bash
# 测试在线订阅
./clash-tester -source "https://example.com/sub?token=xxx"

# 测试本地配置
./clash-tester -source "config.yaml"

# 指定并发数 (默认 5)
./clash-tester -source "config.yaml" -workers 10
```

### 3. 查看报告
程序运行结束后会在控制台输出简报，并在 `result/` 目录下生成详细的 JSON 报告。

---

## 🌐 Server 模式 & API 文档

Server 模式允许你启动一个常驻服务，通过 HTTP 请求对指定节点进行实时测试。这非常适合集成到自动化的节点筛选脚本中。

### 启动服务
```bash
./clash-tester -mode server -port 8080 -workers 5
```

### API 接口

#### `POST /api/v1/test`

执行节点测试。

**请求体 (JSON):**

| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `node` | Object | **必填**。Clash 格式的代理节点配置对象。 |
| `tests` | Array[String] | 选填。指定要测试的项目。支持：`openai`, `gemini`, `claude`, `netflix`, `disney`, `youtube`, `max`。若留空则测试默认集合。 |

**示例 Request:**
```json
{
  "node": {
    "name": "🇺🇸 US Node 01",
    "type": "vless",
    "server": "1.2.3.4",
    "port": 443,
    "uuid": "uuid-string",
    "tls": true,
    "servername": "example.com",
    "network": "ws",
    "ws-opts": {
      "path": "/ws",
      "headers": {
        "Host": "example.com"
      }
    }
  },
  "tests": ["openai", "netflix", "youtube"]
}
```

**响应体 (JSON):**

| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `node_name` | String | 节点名称。 |
| `results` | Object | 测试结果详情。包含各服务的 `available`, `region`, `response_time` 等。 |
| `tags` | Array[String] | 建议的标签列表，如 `["OpenAI", "Netflix"]`。 |
| `error` | String | 如果请求处理失败，返回错误信息。 |

**示例 Response:**
```json
{
  "node_name": "🇺🇸 US Node 01",
  "results": {
    "openai": {
      "service": "openai",
      "available": true,
      "country": "US",
      "status_code": 200,
      "response_time_ms": 230,
      "attempts": 1
    },
    "netflix": {
      "service": "netflix",
      "available": true,
      "region": "US",
      "details": "Full",
      "response_time_ms": 450
    },
    "youtube": {
      "service": "youtube",
      "available": true,
      "region": "US",
      "details": "Premium Available",
      "response_time_ms": 120
    }
  },
  "tags": [
    "OpenAI",
    "Netflix"
  ]
}
```

---

## 🐳 Docker 部署

*(即将推出)*

---

## 🛠️ 构建

```bash
# 依赖 Go 1.21+
go mod tidy
go build -o clash-tester cmd/main.go
```

## 📝 常见问题

**Q: 为什么测试结果显示失败，但我本地能用？**
A: 检测逻辑模拟了真实浏览器的请求头，但部分机场对 IDC IP 有严格的风控。另外，并发测试过快可能导致暂时性被封锁，尝试降低 `-workers` 数量。

**Q: Server 模式下修改配置会重启 Mihomo 吗？**
A: 不会。程序使用了 Mihomo 的 API 动态重载配置，Worker 进程是持久化的，只有配置内容会被热更新，效率较高。

## 📄 License

MIT License
