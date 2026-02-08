# nanobot-go 架构概览

## 组件分层

- **CLI (`cmd/nanobot`)**：统一命令行入口（agent / gateway / cron / bind 等）。
- **Gateway (`internal/cli/gateway`)**：
  - 加载配置、创建 Provider、初始化 Agent Loop
  - 初始化 Message Bus / Channel Registry
  - 启动 Web UI Server（同端口）
- **Agent Loop (`internal/agent`)**：
  - 负责对话轮次与工具调用
  - 调用 `pkg/tools` 完成文件/命令/web 等动作
  - 会话与记忆保存在 workspace 目录
- **Channels (`internal/channels`)**：
  - Telegram（Bot API 轮询）
  - WhatsApp（Bridge WebSocket）
  - Discord（Bot API）
  - WebSocket（自定义接入）
- **Web UI (`webui/`)**：
  - 前端打包后由 Gateway 静态托管
  - 通过 `/api/*` 与后端通讯

## Web Fetch 方案

### HTTP 模式（默认）

- 直接由 Go `net/http` 抓取页面
- 轻量、无额外依赖
- 适合文档/API/静态页面

### 浏览器模式（推荐复杂站点）

为了模拟真实浏览器行为（真实 UA、JS 渲染、反爬策略），使用 **Node + Playwright** 作为可选抓取引擎：

- **实现位置**：`webfetcher/fetch.mjs`
- **工作方式**：
  1. `web_fetch` 工具根据配置判断 `mode=browser`
  2. Go 侧启动 Node 进程，向 `fetch.mjs` 传入 JSON 请求（stdin）
  3. Playwright 打开无头浏览器、加载页面、提取 `document.body.innerText`
  4. Go 侧截断并返回结果

### 配置入口

`~/.nanobot/config.json`：

```json
{
  "tools": {
    "web": {
      "fetch": {
        "mode": "browser",
        "scriptPath": "/absolute/path/to/nanobot-go/webfetcher/fetch.mjs",
        "nodePath": "node",
        "timeout": 30,
        "userAgent": "Mozilla/5.0 ...",
        "waitUntil": "domcontentloaded"
      }
    }
  }
}
```

## WhatsApp / Telegram 绑定

- **WhatsApp**：由 `bridge/` (Baileys) 维护登录态，Gateway 通过 WebSocket 接入。
  - CLI：`nanobot whatsapp bind --bridge ws://localhost:3001`
  - Web UI：状态页显示二维码
- **Telegram**：使用 Bot Token，Web UI 显示 Bot 链接二维码用于快速打开聊天。
