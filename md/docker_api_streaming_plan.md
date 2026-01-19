# Clash-tester Phase 2: Docker 化、API 服务与流媒体检测计划

本文档详细规划了将 Clash-tester 从单纯的命令行工具升级为支持 Docker 部署、提供 HTTP API 服务，并扩展流媒体检测能力的方案。

---

## 1. 总体架构变更

### 当前架构 (CLI Mode)
- **输入**: 命令行参数（URL/文件）。
- **执行**: 启动 N 个临时 Worker -> 批量测试 -> 输出结果 -> 退出。
- **核心**: 仅作为一次性任务运行。

### 目标架构 (Server Mode)
- **输入**: HTTP 请求 (JSON)。
- **执行**: 
    - 维护一个**持久化**的 Worker 资源池（预启动 N 个 Mihomo 实例）。
    - 接收 API 请求，将测试任务分发给空闲 Worker。
    - 实时返回测试结果。
- **部署**: Docker 容器化，支持 Linux (amd64/arm64)。
- **集成**: 提供给 SubStore (Loon/Surge/QuanX) 等外部脚本调用。

---

## 2. 流媒体检测模块扩展

在 `internal/tester/` 中新增流媒体检测逻辑。

| 服务 | 检测 URL (示例) | 判定逻辑 (Head/Get) |
| :--- | :--- | :--- |
| **Netflix** | `https://www.netflix.com/title/81243996` (自家剧集) | 状态码 200 且无重定向至 `/xxx/watch/` (区域限制)；检测 `Location` 头判断是否仅自制剧。 |
| **Disney+** | `https://www.disneyplus.com/login` | 状态码 200，不跳转至 unavailable 页面。 |
| **HBO Max** | `https://www.max.com/` | 状态码 200，GeoIP 判定 + 特定 API 响应。 |
| **Youtube** | `https://www.youtube.com/premium` | 检测是否允许购买 Premium (判断送中/送印等)。 |

**数据结构更新 (`pkg/models/types.go`):**

```go
type StreamTest struct {
    Service   string `json:"service"`    // Netflix, Disney+, etc.
    Available bool   `json:"available"`
    Region    string `json:"region"`     // US, SG, HK, or "Originals Only"
    Details   string `json:"details"`
}
```

---

## 3. API 服务设计

新增 `server` 模式，使用 Go 标准库或轻量级路由启动 HTTP 服务。

### 3.1 启动方式
```bash
./clash-tester -mode server -port 8080 -workers 5
```

### 3.2 接口定义

#### `POST /api/v1/test`
用于 SubStore 脚本调用，接收单个或少量节点配置，返回检测结果。

**Request Body:**
```json
{
  "node": {
    "name": "US Node 1",
    "type": "vless",
    "server": "...",
    "port": 443,
    "uuid": "...",
    "tls": true
    // ... 完整的 clash 代理配置对象
  },
  "tests": ["openai", "netflix", "disney"] // 指定需要测试的项目
}
```

**Response Body:**
```json
{
  "node_name": "US Node 1",
  "results": {
    "openai": { "available": true, "region": "US" },
    "netflix": { "available": true, "region": "US" },
    "disney": { "available": false, "error": "Region blocked" }
  },
  "tags": ["OpenAI", "Netflix(US)"] // 建议的标签
}
```

---

## 4. Docker 化方案

目标是构建轻量级、开箱即用的 Docker 镜像。

### 4.1 目录结构调整
```
/
├── Dockerfile
├── docker-compose.yml
├── entrypoint.sh       # 用于判断架构下载对应 mihomo 或启动
└── ...
```

### 4.2 Dockerfile 规划

采用多阶段构建：

```dockerfile
# Build Stage
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o clash-tester cmd/main.go

# Runtime Stage
FROM alpine:latest
WORKDIR /app

# 安装基础依赖 (ca-certificates 用于 HTTPS, tzdata 用于时区)
RUN apk add --no-cache ca-certificates tzdata curl

# 复制二进制文件
COPY --from=builder /app/clash-tester /app/clash-tester

# 准备 Mihomo (在 entrypoint 或构建时下载)
# 这里假设我们会根据架构自动下载
COPY scripts/get-mihomo.sh /app/
RUN chmod +x /app/get-mihomo.sh && /app/get-mihomo.sh

EXPOSE 8080
ENTRYPOINT ["/app/clash-tester", "-mode", "server"]
```

---

## 5. SubStore 脚本集成 (JavaScript)

这将是用户在 SubStore 中使用的 Script 逻辑。

```javascript
// SubStore Script 伪代码
async function operator(proxies) {
    const API_ENDPOINT = "http://your-docker-ip:8080/api/v1/test";
    
    // 限制并发，避免 API 爆炸
    const BATCH_SIZE = 5;
    
    for (let i = 0; i < proxies.length; i += BATCH_SIZE) {
        const batch = proxies.slice(i, i + BATCH_SIZE);
        await Promise.all(batch.map(async (proxy) => {
            try {
                const resp = await $http.post({
                    url: API_ENDPOINT,
                    body: {
                        node: proxy, // 将 SubStore 代理对象转为 Clash 格式
                        tests: ["openai", "netflix"]
                    }
                });
                
                const res = JSON.parse(resp.body);
                // 根据结果打标签
                if (res.results.openai.available) {
                    proxy.name = `[AI] ${proxy.name}`;
                }
                if (res.results.netflix.available) {
                    proxy.name = `[NF] ${proxy.name}`;
                }
            } catch (e) {
                console.log(`Test failed for ${proxy.name}: ${e}`);
            }
        }));
    }
    return proxies;
}
```

---

## 6. 开发路线图 (Roadmap)

### Phase 2.1: 代码重构与流媒体
1.  **重构 `cmd/main.go`**: 抽离 CLI 逻辑，引入 `server` 启动模式。
2.  **实现 `internal/server`**: 编写 HTTP Handler，管理 Worker Pool。
    *   *难点*: 如何让持久运行的 Mihomo 实例动态接受新的节点配置？
    *   *解法*: Mihomo 支持 Provider API，或者直接复用当前的 `SwitchProxy` 逻辑（每个 Worker 对应一个固定端口的 Mihomo，收到请求后通过 API 动态修改该 Worker 的 Selector 节点或 Provider）。
3.  **实现流媒体检测**: 编写 `internal/tester/netflix.go` 等。

### Phase 2.2: Docker 封装
1.  编写 `Dockerfile`。
2.  编写 `docker-compose.yml`。
3.  在 Linux 环境（或 WSL）下验证运行。

### Phase 2.3: SubStore 对接
1.  编写并测试 JS 脚本。
2.  优化 API 响应速度（调整超时策略）。

---

## 7. 关键技术难点与解决方案

### 问题：Docker 内运行 Mihomo
**风险**: Mihomo 需要 TUN 权限或者网络权限。
**解决**: 在 Docker 中仅使用 HTTP 代理模式，不需要 TUN 模式。只需确保容器内端口不冲突。普通用户权限即可运行 HTTP 模式的 Mihomo。

### 问题：动态测试不同节点
**风险**: 之前的逻辑是生成配置文件重启 Mihomo，API 模式下不能频繁重启。
**解决**: 
1. 启动 Mihomo 时配置一个 `Mixed` 类型的 Provider，指向一个本地文件。
2. 当 API 收到测试请求，将节点配置写入该临时文件。
3. 调用 Mihomo API `PUT /providers/{name}` 强制刷新 Provider。
4. 切换节点进行测试。
5. *或者*: 使用 Mihomo 的 `GLOBAL` 代理组，配合 API 动态创建/注入 Proxy (Mihomo 新版 API 可能支持，或者使用 Provider 更加稳妥)。

### 问题：并发冲突
**解决**: 维护一个 `WorkerQueue`。
- Worker 1 (Port 7891, API 9091) -> 处理 Request A
- Worker 2 (Port 7892, API 9092) -> 处理 Request B
- Request C 进来 -> 等待空闲 Worker。

---

## 8. 下一步行动
建议按照 **流媒体支持 -> API 服务化 -> Docker 打包** 的顺序执行。
