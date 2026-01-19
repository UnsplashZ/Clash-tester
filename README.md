# Clash-tester

一个轻量、高效的节点检测系统，专为流媒体解锁和 AI 服务访问测试设计。采用 **Cron 定时任务 + 静态文件服务** 架构，可与 SubStore 等订阅管理工具完美集成，实现节点的自动化打标。

## ✨ 特性

- **Cron 自动化测试**：定时从订阅源抓取节点并执行全面检测。
- **静态文件分发**：测试结果生成为 `tags.json` Map 格式，通过 Nginx 暴露，极其轻量且读取稳定。
- **解锁检测项**：
  - **AI 服务**：OpenAI (ChatGPT), Google Gemini, Anthropic Claude。
  - **流媒体**：Netflix (区分 Full/Originals), Disney+, YouTube, HBO Max。
- **原子性更新**：采用文件原子移动操作，确保 SubStore 读取数据时永不读取到损坏的中间状态。
- **并发执行**：支持多 Worker 并发测试，大幅缩短大规模订阅的检测时间。
- **多架构支持**：提供 Docker 镜像，支持 `amd64` 和 `arm64` 架构。

---

## 🐳 Docker 快速部署

推荐使用 Docker Compose 部署。

### 1. 配置文件 `docker-compose.yml`

```yaml
version: '3.8'

services:
  # 生产者：负责测试并生成 tags.json
  tester:
    image: ghcr.io/${GITHUB_USERNAME}/clash-tester:latest
    container_name: clash-tester-worker
    restart: unless-stopped
    environment:
      - SUB_URL=https://your-subscription-url.com/sub  # 你的机场订阅地址
      - INTERVAL=3600                                  # 测试间隔 (秒)
      - TZ=Asia/Shanghai
    volumes:
      - shared_data:/data

  # 暴露者：提供静态文件访问
  server:
    image: nginx:alpine
    container_name: clash-tester-server
    restart: unless-stopped
    ports:
      - "8080:80"                                      # 外部访问端口
    volumes:
      - shared_data:/usr/share/nginx/html:ro           # 只读挂载
    depends_on:
      - tester

volumes:
  shared_data:
```

### 2. 启动
```bash
docker-compose up -d
```
启动后，你可以通过 `http://服务器IP:8080/tags.json` 访问生成的测试数据。

---

## 🔗 SubStore 集成

在 SubStore 中添加一个 **脚本操作 (Script Operator)**，内容使用项目中提供的 `substore_js/clash_tester_operator.js`。

### 脚本核心逻辑
1. 定时从你的服务器获取 `tags.json`。
2. 根据节点名称匹配测试结果。
3. 为节点名称添加 `[Chat|NF|YT]` 等前缀标签。

---

## 📊 数据格式说明 (`tags.json`)

系统生成的 Map 格式 JSON 如下，便于根据 Key (节点名) 直接检索：

```json
{
  "🇺🇸 美国 01": {
    "update_time": "2024-01-20T10:00:00Z",
    "openai": { "available": true, "country": "US" },
    "netflix": { "available": true, "region": "US", "result": "Full" },
    "youtube": { "available": true, "region": "US", "premium": true }
  }
}
```

---

## 🛠️ 本地编译

如果你不想使用 Docker，也可以直接本地编译：

```bash
# 1. 下载依赖
go mod download

# 2. 编译
go build -o clash-tester cmd/main.go

# 3. 运行 CLI
# -source: 订阅地址
# -map-output: 生成 map 格式 JSON 的路径
# -mihomo: 指定 mihomo 核心路径
./clash-tester -source "xxx" -map-output "./tags.json" -mihomo "./mihomo" -workers 10
```

---

## 📝 贡献与支持

- **GitHub Actions**: 项目包含手动触发的构建工作流，支持多架构镜像推送。
- **Mihomo Core**: 自动集成最新的 Mihomo 核心，支持 Hysteria2, VLESS, Trojan 等主流协议。

## 📄 License

MIT License