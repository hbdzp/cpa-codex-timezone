# cpa-codex-timezone

[CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)（CPA）插件。CPA 把请求发往 Codex / GPT 模型之前，插件会把请求里的时区和当前日期改成你指定的时区，并提供一个中文 Web 配置界面。

适用于 CPA **v8**（在 v8.0.26 上验证通过）。插件使用 CPA 的 C ABI 动态库插件机制，声明了 `request_normalizer` 和 `management_api` 两种能力。

## 它改了什么

Codex CLI 会在请求中带上客户端环境：

```xml
<environment_context>
  <current_date>2026-10-12</current_date>
  <timezone>America/Los_Angeles</timezone>
</environment_context>
```

当请求的目标协议是 `codex`，并且模型命中 `models` 规则（默认 `gpt-*`）时，插件会：

1. 把 `instructions` 和 `input[]` 中非 assistant 消息里的 `<timezone>` 替换为目标时区。`<timezone status="unavailable" />` 这种形式也会被替换。
2. 把 `<current_date>` 替换为**目标时区下**的今天日期（可关闭）。
3. 按 `inject` 设置，在 `input` 末尾追加一条 developer 消息，告诉模型当地时间：
   - `off`：不追加；
   - `if_missing`（默认）：请求里没有 `<timezone>` 标签时追加，适用于普通 OpenAI SDK / chat completions 客户端；
   - `always`：总是追加。

   追加位置放在末尾，是为了不破坏请求前缀的上游提示缓存。`instructions` 不会被修改。

其他模型、其他上游协议（Claude、Gemini 等）的请求原样转发。

## 构建

需要 Go 1.26+，并开启 cgo（需要 C 编译器）。

```bash
make build    # 输出 bin/codex-timezone.so（macOS 为 .dylib，Windows 为 .dll）
make test     # 单元测试
```

CPA 要求插件 metadata 里的 `GitHubRepository` 不能为空，默认值是 CPA 仓库地址。发布到自己的仓库时可以这样覆盖：

```bash
go build -buildmode=c-shared -ldflags "-X main.pluginRepository=https://github.com/you/cpa-codex-timezone" -o bin/codex-timezone.so .
```

## 安装与配置

1. 把 `bin/codex-timezone.so` 复制到 CPA 的插件目录。**文件名必须是 `codex-timezone`**，CPA 用文件名作为插件 ID。
2. 在 CPA 的 `config.yaml` 中启用：

```yaml
management:
  secret-key: "你的管理密钥"   # Web 界面需要它，留空会关闭管理接口

plugins:
  enabled: true
  dir: "plugins"
  configs:
    codex-timezone:
      enabled: true
      priority: 10
      timezone: "Asia/Shanghai"      # IANA 时区名
      models: ["gpt-*"]              # 模型匹配规则，* 为通配符，不区分大小写
      rewrite_current_date: true     # 同时改写 <current_date>
      inject: "if_missing"           # off | if_missing | always
      inject_template: "Current timezone: {tz}. Current local time: {now}."
```

注入模板支持以下占位符：`{tz}`、`{now}`（`2006-01-02 15:04:05 +08:00`）、`{date}`、`{time}`、`{offset}`、`{weekday}`。

时区名称无效时，CPA 会拒绝这次配置，插件继续使用上一份有效配置。

## Web 配置界面

CPA 启动后，在浏览器打开：

```
http://<CPA 地址>:<端口>/v0/resource/plugins/codex-timezone/ui
```

输入 management key 后可以：

- 按地区（常用、亚洲、欧洲、美洲、大洋洲、中东 / 非洲、UTC）从 80 多个预设时区中选择，每个预设显示中文城市名和当前 UTC 偏移（会跟随夏令时变化）；
- 也可以在输入框里搜索任意 IANA 时区，支持输入中文城市名（如“东京”“纽约”）后自动转换为 IANA 名称，并实时查看该时区的当前时间；
- 编辑模型匹配规则、注入模式和模板；
- 粘贴一个请求体，预览改写前后的差异（不会发出真实请求）；
- 点击“保存并生效”。界面调用 CPA 的 `PATCH /v8/management/config/plugins/configs/codex-timezone` 写入 `config.yaml`，CPA 热重载后无需重启即可生效；
- 查看运行状态：经过插件、命中、改写、注入的请求计数。

页面本身不需要鉴权，但所有读写操作都需要 management key。密钥只保存在浏览器 localStorage 中。

插件额外注册了两个需要鉴权的接口：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/v0/management/codex-timezone/status` | 当前生效配置、当地时间、请求计数 |
| POST | `/v0/management/codex-timezone/preview` | `{"body": "<请求体 JSON>", "model": "可选", "settings": {可选的临时配置}}`，试运行改写 |

## 端到端测试

`scripts/e2e/` 会启动一个记录请求体的 mock Codex 上游，以及加载本插件的真实 CPA。它会验证以下几点：Codex 请求被改写、chat completions 请求被注入说明、不匹配的模型原样转发、通过管理接口保存的配置会热生效。

```bash
git clone --branch v8.0.26 https://github.com/router-for-me/CLIProxyAPI.git
(cd CLIProxyAPI && go build -o cli-proxy-api ./cmd/server)
CPA_BIN=$PWD/CLIProxyAPI/cli-proxy-api make e2e
```

测试用的 CPA 监听 `127.0.0.1:48317`，management key 为 `e2e-management-key`。

## 项目结构

| 路径 | 说明 |
| --- | --- |
| `main.go` | C ABI 导出（`cliproxy_plugin_init` 等） |
| `plugin.go` | 方法分发、注册信息、`request.normalize` |
| `management.go` | Web 界面资源、status / preview 接口 |
| `internal/config` | 配置解析、默认值、校验、模型匹配 |
| `internal/rewrite` | 时区 / 日期改写与注入逻辑 |
| `internal/ui/index.html` | 内嵌的配置页面 |
| `scripts/e2e` | mock 上游、测试配置与端到端脚本 |
