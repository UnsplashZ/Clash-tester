# Clash-Tester: Docker Cron 模式改造计划

## 1. 核心目标
将项目从 HTTP API Server 模式转变为 **Cron 定时任务 + 静态文件服务** 模式。

### 架构优势
* **稳定性**: 测试进程崩溃不影响文件服务。
* **解耦**: 生产者（测试）与消费者（SubStore）完全分离。
* **兼容性**: 静态 JSON 文件适配所有支持 HTTP GET 的脚本工具。

---

## 2. 代码改造计划

### 2.1 移除 Server 模块
* 删除 `internal/server/` 目录。
* 清理 `cmd/main.go` 中关于 server 模式的 flag 和启动逻辑。

### 2.2 优化数据模型与报告生成
目标 JSON 格式：
```json
{
  "Node Name A": {
    "update_time": "...",
    "openai": { "available": true, ... },
    "netflix": { "available": true, "result": "Full" }
  }
}
```

* **修改 `internal/reporter/json.go`**:
    * 新增 `SaveTagMapJSON` 函数。
    * 将 `TestReport` 转换为 `map[string]NodeTagData` 结构。
* **数据结构调整**:
    * 确保 `NodeTagData` 包含 SubStore 脚本所需的精简字段。

### 2.3 调整 CLI 逻辑 (`cmd/main.go`)
* 仅保留 CLI 模式。
* 增加 `-map-output` 参数（可选，或直接替换默认输出），指定生成的 tags.json 路径。
* 确保程序退出码正确：
    * 0: 成功生成。
    * 1: 严重错误（配置加载失败等）。

---

## 3. Docker 化实施

### 3.1 编写 `entrypoint.sh`
* 核心循环脚本。
* 逻辑：`while true; do ./clash-tester ...; sleep $INTERVAL; done`。
* 关键点：使用临时文件 `tags.json.tmp` + `mv` 实现原子写入。

### 3.2 编写 `Dockerfile`
* **基础镜像**: `alpine:latest` (体积小)。
* **多阶段构建**: 使用 `golang:1.23-alpine` 编译。
* **Mihomo 集成**:
    * 自动下载脚本：根据架构 (amd64/arm64) 下载对应的 Mihomo Alpha 或 Release 版本。
    * *注意*: Mihomo 的文件名和解压路径处理。

### 3.3 编写 `docker-compose.yml`
* 定义两个服务：
    1.  `worker`: 运行测试脚本。
    2.  `server`: `nginx:alpine` 挂载数据卷。

---

## 4. SubStore 脚本适配
* 编写新的 `substore_js/tag_injector.js`，适配 Map 结构的 JSON 数据。

---

## 5. 执行步骤

1.  **代码清理**: 删除 `internal/server`，恢复 `cmd/main.go` 到纯 CLI 模式（但保留并发 Worker 逻辑）。
2.  **报告升级**: 实现 Map 格式的 JSON 导出。
3.  **Docker 构建**: 创建 Dockerfile 和相关脚本。
4.  **脚本编写**: 更新 SubStore JS。

## 6. 兼容性注意事项 (Mihomo)
* **下载源**: 使用 GitHub Releases (`MetaCubeX/mihomo`)。
* **配置兼容**: 确保生成的 `config.yaml` 兼容 Mihomo 最新版（特别是 `external-controller` 和 `proxies` 字段）。
* **架构检查**: Dockerfile 需正确处理 `TARGETARCH`。
