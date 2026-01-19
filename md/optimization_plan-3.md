# Clash-Tester: 自动化节点检测与分流系统 (Cron 模式)

## 1. 项目概述

本项目旨在解决 SubStore 等订阅管理工具无法实时、精准地获取节点流媒体解锁情况的问题。通过“生产者-消费者”架构，实现节点检测与订阅生成的解耦。

### 核心设计理念
* **解耦 (Decoupling)**: 检测端 (`tester`) 只负责生成数据，服务端 (`server`) 只负责暴露数据，消费端 (`SubStore`) 只负责读取数据。
* **原子性 (Atomicity)**: 采用文件原子移动操作，确保读取端永远不会读取到写入一半的损坏数据。
* **无状态 (Stateless)**: 容器重启不丢失关键配置，且不需要复杂的数据库支持。

---

## 2. 系统架构



[Image of automated testing workflow]


系统由两个 Docker 容器组成，通过共享 Volume 进行数据交换。

### 2.1 生产者: `clash-tester-worker`
* **运行模式**:基于 Alpine 的 Cron 定时任务。
* **职责**:
    1.  定时（如每 1 小时）从机场订阅 URL 下载最新节点配置。
    2.  启动轻量级 Mihomo 核心进行节点连通性测试。
    3.  针对指定节点执行 7 项流媒体/AI 服务检测 (OpenAI, Netflix, Disney+, etc.)。
    4.  生成结果并写入共享目录。

### 2.2 暴露者: `clash-tester-server`
* **运行模式**: Nginx (Alpine)。
* **职责**:
    1.  挂载共享目录为 Web 根目录。
    2.  提供 HTTP GET 接口供外部访问 `tags.json`。
    3.  配置 HTTP 头以禁用浏览器缓存，确保数据实时性。

---

## 3. 输出数据定义 (JSON Contract)

这是系统对外的核心交付物。文件路径为 `/data/tags.json`。

### JSON 结构示例

```json
{
  "🇺🇸 美国节点 01 [高速]": {
    "update_time": "2024-01-20T10:00:00Z",
    "openai": {
      "available": true,
      "region": "US"
    },
    "gemini": {
      "available": true,
      "region": "US"
    },
    "claude": {
      "available": false,
      "error": "Region blocked"
    },
    "netflix": {
      "available": true,
      "region": "US",
      "result": "Full"
    },
    "disney": {
      "available": true,
      "region": "US"
    },
    "max": {
      "available": true,
      "region": "US"
    },
    "youtube": {
      "available": true,
      "region": "US",
      "premium": false
    }
  },
  "🇭🇰 香港节点 02": {
    "update_time": "2024-01-20T10:05:00Z",
    "openai": { "available": true, "region": "HK" },
    "gemini": { "available": true, "region": "HK" },
    "claude": { "available": true, "region": "HK" },
    "netflix": { "available": true, "region": "HK", "result": "Originals" },
    "disney": { "available": true, "region": "HK" },
    "max": { "available": false, "error": "GeoIP Block" },
    "youtube": { "available": true, "region": "HK", "premium": true }
  }
}

```

### 字段说明

* **Key**: 节点原始名称 (建议结合 Server+Port Hash 以处理重名)。
* **netflix.result**:
* `Full`: 完整解锁（非自制剧可用）。
* `Originals`: 仅自制剧可用。
* `None` / `Blocked`: 不可用。


* **update_time**: 该节点最后一次测试完成的时间。

---

## 4. 部署方案 (Docker Compose)

### 4.1 目录结构

```text
.
├── docker-compose.yml
├── entrypoint.sh          # 生产者的启动脚本
├── nginx.conf             # (可选) Nginx 配置文件
└── clash-tester           # (编译后的二进制文件或源码)

```

### 4.2 `entrypoint.sh` (核心循环逻辑)

```bash
#!/bin/sh

echo "Starting Clash-Tester Cron Service..."
echo "Target Subscription: $SUB_URL"
echo "Check Interval: $INTERVAL seconds"

# 确保输出目录存在
mkdir -p /data

while true; do
    echo "[$(date)] 🔄 Starting new test cycle..."
    
    # 1. 执行测试
    # -output 指向临时文件，实现原子写入
    ./clash-tester -mode cli -url "$SUB_URL" -output "/data/tags.json.tmp"
    
    EXIT_CODE=$?
    
    if [ $EXIT_CODE -eq 0 ] && [ -f "/data/tags.json.tmp" ]; then
        # 2. 原子移动 (Atomic Move)
        # 即使 SubStore 正在读取 tags.json，mv 操作也是瞬间完成的，不会读到半截数据
        mv /data/tags.json.tmp /data/tags.json
        echo "[$(date)] ✅ Test finished. JSON updated."
    else
        echo "[$(date)] ❌ Test failed or no output generated."
        # 失败不覆盖旧文件，保留上次成功的结果
    fi
    
    # 3. 等待下一次周期
    echo "[$(date)] 💤 Sleeping for $INTERVAL seconds..."
    sleep $INTERVAL
done

```

### 4.3 `docker-compose.yml`

```yaml
version: '3.8'

services:
  # ------------------------------------------------
  # 1. 生产者：定时测试节点并生成 JSON
  # ------------------------------------------------
  tester:
    build: .                 # 基于 Dockerfile 构建包含 mihomo 和 clash-tester 的镜像
    container_name: clash-tester-worker
    restart: unless-stopped
    environment:
      - SUB_URL=[https://example.com/api/v1/client/subscribe?token=your_token](https://example.com/api/v1/client/subscribe?token=your_token)
      - INTERVAL=3600        # 默认每 1 小时执行一次
      - TZ=Asia/Shanghai
    volumes:
      - shared_data:/data    # 写入共享卷
    entrypoint: ["/app/entrypoint.sh"]

  # ------------------------------------------------
  # 2. 暴露者：提供 HTTP 访问接口
  # ------------------------------------------------
  server:
    image: nginx:alpine
    container_name: clash-tester-server
    restart: unless-stopped
    ports:
      - "8080:80"            # 外部端口 8080
    volumes:
      - shared_data:/usr/share/nginx/html:ro  # 只读挂载
      # 推荐：挂载自定义配置以禁用缓存
      # - ./nginx_no_cache.conf:/etc/nginx/conf.d/default.conf:ro
    depends_on:
      - tester

volumes:
  shared_data:

```

---

## 5. SubStore 接入指南

在 SubStore 中，你需要创建一个 **Script Operator (脚本操作)** 来消费上述服务生成的 JSON。

### 脚本代码 (`tag_injector.js`)

```javascript
/**
 * @name Clash-Tester Tag Injector
 * @description 读取外部 JSON 结果，自动为节点添加 [AI][NF] 等流媒体标签
 */

async function operator(proxies) {
    // Docker 宿主机 IP 或容器网络中的别名 (如 http://clash-tester-server/tags.json)
    const API_URL = "[http://192.168.1.100:8080/tags.json](http://192.168.1.100:8080/tags.json)"; 
    
    let tagsMap = {};
    
    try {
        const resp = await $http.get(API_URL);
        tagsMap = JSON.parse(resp.body);
    } catch (e) {
        console.log("⚠️ 无法获取测试结果，将跳过打标: " + e);
        return proxies; // 容错：获取失败返回原列表
    }

    return proxies.map(p => {
        const data = tagsMap[p.name];
        
        // 如果该节点没有测试记录，直接返回原节点
        if (!data) return p;

        let tags = [];

        // --- 1. AI 服务 ---
        if (data.openai?.available) tags.push("AI");
        else if (data.claude?.available) tags.push("Claude");
        else if (data.gemini?.available) tags.push("Gemini");

        // --- 2. 流媒体 (Netflix) ---
        if (data.netflix?.available) {
            let nfTag = "NF";
            if (data.netflix.result === "Originals") nfTag = "NF(自制)";
            // 可选：添加地区后缀
            // if (data.netflix.region) nfTag += `.${data.netflix.region}`;
            tags.push(nfTag);
        }

        // --- 3. YouTube ---
        if (data.youtube?.available) {
            tags.push("YT");
            if (data.youtube.premium) tags.push("YTP");
        }

        // --- 4. 修改名称 ---
        // 原始: "香港节点 01"
        // 修改: "[AI][NF][YT] 香港节点 01"
        if (tags.length > 0) {
            const prefix = tags.map(t => `[${t}]`).join("");
            p.name = `${prefix} ${p.name}`;
        }

        return p;
    });
}

```

### Mihomo 分流配置建议

在打标完成后，你可以在 Mihomo (Clash) 的 `proxy-groups` 中使用正则表达式轻松分流：

```yaml
proxy-groups:
  - name: 🤖 OpenAI
    type: url-test
    filter: "(?i)\\[AI\\]"  # 匹配带有 [AI] 标签的节点
    use: [SubStore]

  - name: 🎥 Netflix
    type: select
    filter: "(?i)\\[NF"     # 匹配 [NF] 或 [NF(自制)]
    use: [SubStore]