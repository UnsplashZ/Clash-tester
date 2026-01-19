# Clash-tester 环境准备与安装指南 (Windows)

## ❌ 环境检查结果

1.  **Go 语言环境**: 未检测到 (`go` 命令不存在)。项目需要 Go 1.21+。
2.  **Mihomo Core**: 未检测到 (`mihomo` 命令不存在)。项目依赖它作为代理核心。

---

## 📥 Windows 安装流程

### 1. 安装 Go 语言环境

*   **下载**: 访问 [Go 官方下载页](https://go.dev/dl/)，下载 Windows 版本的安装包 (例如 `go1.23.4.windows-amd64.msi`)。
*   **安装**: 运行安装包，一路点击 "Next" 即可。默认会安装到 `C:\Program Files\Go` 并自动配置环境变量。
*   **验证**: 安装完成后，**需要重启你的终端/命令行窗口**（VS Code 也需要重启），然后运行：
    ```powershell
    go version
    ```

### 2. 下载与配置 Mihomo (Clash Core)

项目计划中使用 `mihomo` 核心来处理具体的代理协议连接。

*   **下载**: 访问 [Mihomo Releases](https://github.com/MetaCubeX/mihomo/releases)。
    *   下载最新版本的 `windows-amd64` 版本 (例如 `mihomo-windows-amd64-v1.18.x.zip`)。
*   **安装**:
    1.  解压下载的 zip 文件。
    2.  将解压出来的 `.exe` 文件重命名为 **`mihomo.exe`**。
    3.  **建议方式**: 将 `mihomo.exe` 放入你的项目根目录 `C:\Users\zheng\Documents\Github\Clash-tester\` 下。

### 3. 初始化项目 (环境准备好后)

当你安装好 Go 并重启终端后，你可以开始初始化项目：

```powershell
# 1. 初始化 Go 模块
go mod init Clash-tester

# 2. 获取依赖 (根据计划文档)
go get gopkg.in/yaml.v3
go get golang.org/x/net/proxy

# 3. 创建目录结构 (Windows Powershell)
md cmd, internal, pkg, configs, result
md internal\config, internal\parser, internal\proxy, internal\tester, internal\reporter
md pkg\models
```
