# cpa-codex-timezone

[CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)（下文简称 CPA）的时区插件：在 CPA 把请求转发给 Codex / GPT 模型之前，把请求里的**时区**和**当前日期**改成你指定的时区，并提供一个中文 Web 配置界面。

典型场景：CPA 部署在海外服务器，或者客户端系统时区不对，模型就会按错误的时区理解“今天”和“现在几点”。装上插件后，模型看到的始终是你设置的时区，例如 `Asia/Shanghai`。

- 适用于 CPA **v8**（在 v8.0.26 上验证通过）
- 只处理发往 Codex 上游、模型名匹配规则（默认 `gpt-*`）的请求，其他请求原样转发
- 配置可在 Web 界面修改，保存后热生效，不用重启 CPA

---

## 目录

- [它做了什么](#它做了什么)
- [环境要求](#环境要求)
- [快速开始](#快速开始)
- [编译](#编译)
- [安装](#安装)
- [配置](#配置)
- [确认插件已生效](#确认插件已生效)
- [使用 Web 配置界面](#使用-web-配置界面)
- [客户端接入](#客户端接入)
- [升级与卸载](#升级与卸载)
- [常见问题](#常见问题)
- [开发与测试](#开发与测试)

---

## 它做了什么

**1. 改写 Codex CLI 的环境信息。** Codex CLI 每次请求都会附带客户端环境，例如：

```xml
<environment_context>
  <cwd>/home/me/project</cwd>
  <current_date>2026-10-12</current_date>
  <timezone>America/Los_Angeles</timezone>
</environment_context>
```

插件把它改成（以 `Asia/Shanghai` 为例，此时上海已是 10 月 13 日）：

```xml
<environment_context>
  <cwd>/home/me/project</cwd>
  <current_date>2026-10-13</current_date>
  <timezone>Asia/Shanghai</timezone>
</environment_context>
```

- 改写范围是 `instructions` 和 `input[]` 中所有非 assistant 的消息，`<timezone status="unavailable" />` 这种写法也会被替换。
- `<current_date>` 按目标时区重新计算，这一项可以关闭。

**2. 为没有环境信息的客户端补充当地时间。** 普通 OpenAI SDK、`/v1/chat/completions` 等请求里没有上面这段内容。默认情况下（`inject: if_missing`），插件会在 `input` 末尾追加一条 developer 消息：

```text
Current timezone: Asia/Shanghai. Current local time: 2026-10-13 00:10:34 +08:00.
```

追加在末尾，是为了不破坏上游的提示缓存；`instructions` 不会被修改。

**3. 其他请求原样转发。** 模型不匹配规则的请求，以及发往 Claude、Gemini 等其他上游的请求，插件都不会修改。

插件作用于 CPA 的“请求规整”环节，也就是协议转换之后、发往上游之前，所以下面三类请求都会生效：

- Codex CLI 直接发来的 Responses 请求；
- OpenAI chat completions 请求经 CPA 转换后的结果；
- 其他格式经 CPA 转换后的结果。

---

## 环境要求

| 项目 | 要求 |
| --- | --- |
| CPA | v8.x，需要开启 `plugins.enabled` |
| 编译工具 | Go 1.26 及以上，并且需要 C 编译器（插件使用 cgo 编译为动态库） |
| Linux 运行环境 | glibc 2.34 及以上。Debian 12、Ubuntu 22.04+ 和 CPA 官方 Docker 镜像都满足；Alpine（musl）不支持 |
| Web 界面 | CPA 需要设置 `management.secret-key` |

各平台安装 C 编译器的方法：

- **Debian / Ubuntu**：`sudo apt install build-essential`
- **macOS**：`xcode-select --install`
- **Windows**：安装 [MSYS2](https://www.msys2.org/) 后执行 `pacman -S mingw-w64-ucrt-x86_64-gcc`，并把 `C:\msys64\ucrt64\bin` 加入 `PATH`

> 插件必须在与 CPA **相同的操作系统和 CPU 架构**上编译，例如 Linux amd64 的 CPA 要用 Linux amd64 编译出的 `.so`。cgo 不方便交叉编译，最省事的做法是在目标机器上编译，或者使用下文的 Docker 编译方式。

---

## 快速开始

以 Linux 上直接运行的 CPA 为例，CPA 工作目录为 `/opt/cliproxyapi`：

```bash
# 1. 编译插件
git clone <本仓库地址> cpa-codex-timezone
cd cpa-codex-timezone
make dist                # 产物：bin/codex-timezone-v0.1.0.so

# 2. 放入 CPA 的插件目录
mkdir -p /opt/cliproxyapi/plugins
cp bin/codex-timezone-v0.1.0.so /opt/cliproxyapi/plugins/
```

然后编辑 CPA 的 `config.yaml`，加入以下内容（已有 `management`、`plugins` 段的话就合并进去）：

```yaml
management:
  secret-key: "换成你自己的管理密钥"

plugins:
  enabled: true
  dir: "plugins"
  configs:
    codex-timezone:
      enabled: true
      priority: 10
      timezone: "Asia/Shanghai"
```

最后重启 CPA，在浏览器打开 `http://<CPA地址>:8317/v0/resource/plugins/codex-timezone/ui`，输入管理密钥即可。

---

## 编译

| 命令 | 产物 | 说明 |
| --- | --- | --- |
| `make dist` | `bin/codex-timezone-v<版本>.<扩展名>` | **推荐。** 文件名带版本号，升级时可热替换 |
| `make build` | `bin/codex-timezone.<扩展名>` | 不带版本号 |
| `make docker-build` | `bin/linux/amd64/codex-timezone-v<版本>.so` | 在 `golang:1.26-bookworm` 容器里编译，与 CPA 官方镜像的系统一致 |

扩展名随系统而定：Linux 为 `.so`，macOS 为 `.dylib`，Windows 为 `.dll`。

可选参数：

```bash
make dist VERSION=0.2.0                                          # 设置版本号
make dist REPOSITORY=https://github.com/you/cpa-codex-timezone   # 设置插件信息中的仓库地址
make docker-build DOCKER_PLATFORM=linux/arm64                    # 为 arm64 编译（需要 Docker 支持多架构）
```

> CPA 要求插件信息里的仓库地址不能为空，默认值是 CPA 自己的仓库地址。发布到自己的仓库时，建议用 `REPOSITORY` 改成你的地址。

**没有 `make` 时**（例如 Windows），可以直接运行：

```bash
go build -buildmode=c-shared -trimpath -ldflags "-s -w" -o codex-timezone-v0.1.0.dll .
```

编译时会生成一个同名的 `.h` 头文件，没有用处，可以删掉。

---

## 安装

### 插件文件命名

CPA 根据文件名识别插件，规则如下：

- `codex-timezone.so`：插件 ID 为 `codex-timezone`，不带版本号；
- `codex-timezone-v0.1.0.so`：插件 ID 为 `codex-timezone`，版本号为 `0.1.0`。

**插件 ID 必须是 `codex-timezone`**，与 `plugins.configs` 下的键名一致。不要把文件改成别的名字。

### 直接运行的 CPA

把插件文件放到 `plugins.dir` 指定的目录：

- 默认值是 `plugins`，这是**相对 CPA 进程启动时的工作目录**的路径，不是相对配置文件所在目录。如果不确定，可以写成绝对路径，例如 `dir: "/opt/cliproxyapi/plugins"`，也支持 `~/` 开头的路径。
- CPA 会依次在 `<dir>/<系统>/<架构>/` 和 `<dir>/` 两个位置查找插件，例如 `plugins/linux/amd64/codex-timezone-v0.1.0.so`。需要同时部署多平台时可以用前一种目录结构。

### Docker / docker-compose 部署的 CPA

CPA 官方的 `docker-compose.yml` 已经把宿主机的 `./plugins` 目录挂载到容器内的 `/CLIProxyAPI/plugins`。你也可以用环境变量 `CLI_PROXY_PLUGIN_PATH` 指定宿主机上的其他目录。

```bash
make docker-build                    # 编译出与容器系统一致的 .so
cp bin/linux/amd64/codex-timezone-v0.1.0.so <docker-compose 所在目录>/plugins/
```

然后在挂载进容器的 `config.yaml` 中按下一节进行配置，并执行 `docker compose restart`。

> 如果宿主机本身就是 Debian 12 或 Ubuntu 22.04 以上的系统，在宿主机上直接 `make dist` 编译出的 `.so` 也能在官方镜像中运行。

---

## 配置

完整配置示例（写在 CPA 的 `config.yaml` 中）：

```yaml
management:
  secret-key: "换成你自己的管理密钥"   # Web 界面需要；留空会关闭整个管理接口
  allow-remote: false                 # 需要从其他机器打开 Web 界面时改为 true

plugins:
  enabled: true                       # CPA 插件总开关
  dir: "plugins"
  configs:
    codex-timezone:
      enabled: true                   # 本插件开关
      priority: 10                    # 多个插件改写同一请求时，数值大的先执行
      timezone: "Asia/Shanghai"
      models: ["gpt-*"]
      rewrite_current_date: true
      inject: "if_missing"
      inject_template: "Current timezone: {tz}. Current local time: {now}."
```

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `enabled` | `false` | 是否启用本插件 |
| `timezone` | `Asia/Shanghai` | IANA 时区名，例如 `Asia/Tokyo`、`Europe/London`、`UTC` |
| `models` | `["gpt-*"]` | 需要改写的模型；`*` 为通配符，不区分大小写。例如 `["gpt-5*", "o3"]` |
| `rewrite_current_date` | `true` | 是否同时按目标时区改写 `<current_date>` |
| `inject` | `if_missing` | 何时追加当地时间说明，取值见下方 |
| `inject_template` | 见上 | 追加说明的内容模板 |

`inject` 的取值：

- `off`：从不追加；
- `if_missing`：请求中没有 `<timezone>` 标签时追加；
- `always`：总是追加。

模板占位符：

| 占位符 | 示例 |
| --- | --- |
| `{tz}` | `Asia/Shanghai` |
| `{now}` | `2026-10-13 00:10:34 +08:00` |
| `{date}` | `2026-10-13` |
| `{time}` | `00:10:34` |
| `{offset}` | `+08:00` |
| `{weekday}` | `Tuesday` |

说明：

- 时区名无效或 `inject` 取值不合法时，CPA 会拒绝这份配置，插件继续使用上一份有效配置，CPA 日志中会有对应的警告。
- 插件内置了完整的时区数据库，不依赖系统的 zoneinfo，在 Windows 上也能识别所有 IANA 时区。
- 修改 `config.yaml` 后 CPA 会自动热重载，不用重启。

---

## 确认插件已生效

**1. 查看 CPA 启动日志**，出现以下两行说明插件加载成功：

```text
pluginhost: plugin loaded plugin_id=codex-timezone version=0.1.0 path=plugins/codex-timezone-v0.1.0.so
pluginhost: plugin registered plugin_id=codex-timezone plugin_name=codex-timezone version=0.1.0 ...
```

**2. 调用状态接口：**

```bash
curl -H "Authorization: Bearer <管理密钥>" http://127.0.0.1:8317/v0/management/codex-timezone/status
```

返回内容包括当前生效的配置、当地时间和请求计数。`seen` 是经过插件的请求数，`matched` 是命中规则的请求数，`rewritten` 是实际被改写的请求数，`injected` 是追加了说明的请求数。

**3. 实际验证：** 让模型回答“现在几点？今天几号？”，答案应该是目标时区的时间。

---

## 使用 Web 配置界面

浏览器打开：

```text
http://<CPA地址>:<端口>/v0/resource/plugins/codex-timezone/ui
```

CPA 默认端口是 `8317`。从其他机器访问时，需要把 `management.allow-remote` 设为 `true`，并且确认 `server.host` 没有绑定在 `127.0.0.1` 上。

使用步骤：

1. **连接**：在顶部输入管理密钥，点击“连接”。连接成功后，右上角会显示“已启用 · 当前时区”。密钥只保存在当前浏览器中，可以点“清除”删除。
2. **选择时区**，有三种方式：
   - 点击地区标签页（常用、亚洲、欧洲、美洲、大洋洲、中东 / 非洲、UTC），从 80 多个预设中选择。每个预设显示中文城市名和当前 UTC 偏移，偏移会随夏令时自动变化；
   - 在输入框中输入 IANA 名称，下拉列表包含全部时区；
   - 直接输入中文城市名，如“东京”“纽约”，输入框失去焦点后会自动转换为 IANA 名称。

   下方时钟会实时显示该时区的当前时间。时区名无效时，输入框变红，“保存并生效”按钮不可点击。
3. **匹配模型**：输入模型名或通配符后回车添加，点 `×` 删除。
4. **注入设置**：选择“关闭”“缺少时注入”或“总是注入”，并可以修改模板，下方会实时显示渲染效果。
5. **预览**（可选）：在“改写预览”中点击“填入示例”，或者粘贴一个真实的请求体，再点击“预览改写”。界面会用左侧**尚未保存**的设置试运行，并左右对比显示改写前后的内容。这一步不会发出真实请求。
6. **保存**：点击“保存并生效”。配置会写入 CPA 的 `config.yaml`，CPA 热重载后界面会提示“已生效”，右侧“运行状态”中的生效时区也会随之更新。

右侧的“运行状态”每 15 秒自动刷新一次，计数从 CPA 加载插件时开始累计。

> 除了本插件的界面，也可以在 CPA 自带的管理面板中，于 `plugins.configs.codex-timezone` 下修改这些字段。

---

## 客户端接入

插件对客户端完全透明。只要客户端通过 CPA 请求 GPT / Codex 模型就会生效，客户端不需要做任何改动。

### Codex CLI

在 `~/.codex/config.toml` 中把 CPA 设为模型提供方：

```toml
model = "gpt-5.5"
model_provider = "cpa"

[model_providers.cpa]
name = "CLIProxyAPI"
base_url = "http://127.0.0.1:8317/v1"
wire_api = "responses"
env_key = "CPA_API_KEY"
```

```bash
export CPA_API_KEY="<CPA access.api-keys 中的某个 key>"
codex
```

Codex CLI 发出的 `<environment_context>` 会被改写，不会追加额外消息。

### OpenAI SDK、curl 及其他客户端

```bash
curl http://127.0.0.1:8317/v1/chat/completions \
  -H "Authorization: Bearer <CPA access.api-keys 中的某个 key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.5","messages":[{"role":"user","content":"现在几点？今天几号？"}]}'
```

这类请求没有环境信息，`inject: if_missing` 时插件会追加一条当地时间说明，模型就能按目标时区回答。

---

## 升级与卸载

### 升级（无需重启）

1. 用新版本号编译，例如 `make dist VERSION=0.2.0`。
2. 把新文件 `codex-timezone-v0.2.0.so` 放进插件目录，**删除旧文件** `codex-timezone-v0.1.0.so`。
3. 触发一次配置重载：在 Web 界面点一次“保存并生效”，或者随便修改并保存一下 `config.yaml`。

CPA 日志中出现 `plugin hot reloaded ... active_version=0.2.0` 就说明升级完成。

> 如果用的是不带版本号的文件名（`codex-timezone.so`），直接覆盖后需要**重启 CPA** 才能加载新版本。

如果想固定使用某个版本，可以在配置中加上 `store: { version: "0.1.0" }`。这是 CPA 插件商店的版本选择字段，同一目录下有多个版本时会按它选择。

### 临时停用

在 Web 界面关闭“启用插件”并保存，或者在 `config.yaml` 中设置 `plugins.configs.codex-timezone.enabled: false`。停用后，所有请求都原样转发。

### 卸载

1. 删除 `config.yaml` 中的 `plugins.configs.codex-timezone` 段；
2. 删除插件目录里的 `codex-timezone*.so`（或 `.dylib` / `.dll`）；
3. 重启 CPA。

---

## 常见问题

| 现象 | 原因与解决 |
| --- | --- |
| 日志里没有 `plugin loaded` | 可能的原因有：`plugins.enabled` 或 `plugins.configs.codex-timezone.enabled` 不是 `true`；插件不在 `plugins.dir` 中（相对路径是相对 CPA 的工作目录）；扩展名与当前系统不符；文件名改过，导致插件 ID 与 `plugins.configs` 下的键名不一致（文件名必须是 `codex-timezone` 或 `codex-timezone-v<版本>`） |
| 日志提示 `returned invalid metadata or no capabilities` | 插件信息里的仓库地址为空，用默认配置或 `REPOSITORY=...` 重新编译即可 |
| 加载时报 `GLIBC_2.34 not found` | 运行环境的 glibc 太旧（如 CentOS 7、Debian 11），需要在目标机器上重新编译 |
| 加载时报 `wrong ELF class` / `cannot open shared object` | 系统或 CPU 架构不一致（例如 arm64 服务器用了 amd64 的 .so），需要在目标平台上编译 |
| 在 Alpine 镜像中加载失败 | Alpine 使用 musl，不支持本插件，请改用基于 Debian 的官方镜像 |
| Web 界面打不开（404） | 插件未加载，或者 CPA 处于 Home 模式 |
| Web 界面提示“管理密钥无效” | 密钥不对；`management.secret-key` 为空；或者从远程访问但 `allow-remote` 为 `false` |
| 保存后一直提示“等待生效” | 配置文件已写入，但 CPA 拒绝了新配置（例如时区名无效），查看 CPA 日志中的 `plugin.reconfigure` 警告 |
| `matched` 一直为 0 | 模型名不匹配 `models` 规则，例如带了前缀 `team/gpt-5.5`，可以把规则改成 `*gpt-*`；或者请求没有发往 Codex 上游 |
| `matched` 增加但 `rewritten` 不增加 | 请求中没有时区标签，并且 `inject` 为 `off` |
| 模型回答的时间仍然不对 | 查看状态接口中的生效时区；也可以用 Web 界面的“改写预览”粘贴真实请求体，检查改写结果 |

---

## 开发与测试

```bash
make test     # 单元测试：改写规则、配置校验、插件方法、页面中预设时区的有效性
make vet
```

### 端到端测试

端到端测试会启动一个记录请求体的模拟 Codex 上游，以及一个加载了本插件的真实 CPA，然后验证以下几点：

- Codex 请求中的时区被改写；
- chat completions 请求被追加了说明；
- 不匹配的模型原样转发；
- 通过管理接口保存的配置能热生效。

```bash
git clone --branch v8.0.26 https://github.com/router-for-me/CLIProxyAPI.git
(cd CLIProxyAPI && go build -o cli-proxy-api ./cmd/server)
CPA_BIN=$PWD/CLIProxyAPI/cli-proxy-api make e2e
```

测试用的 CPA 监听 `127.0.0.1:48317`，管理密钥为 `e2e-management-key`，客户端 key 为 `e2e-client-key`。测试结束后，`.e2e/` 目录中保留了 CPA 日志和上游实际收到的请求（`upstream.jsonl`）。

### 插件提供的管理接口

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/v0/resource/plugins/codex-timezone/ui` | 否 | Web 配置界面 |
| GET | `/v0/management/codex-timezone/status` | 管理密钥 | 生效配置、当地时间、请求计数 |
| POST | `/v0/management/codex-timezone/preview` | 管理密钥 | 试运行改写，见下方说明 |

`preview` 的请求体格式为 `{"body": "<请求体 JSON 字符串>", "model": "可选", "settings": {可选的临时配置}}`。

保存配置使用的是 CPA 自带的 `PATCH /v8/management/config/plugins/configs/codex-timezone` 接口。

### 项目结构

| 路径 | 说明 |
| --- | --- |
| `main.go` | C ABI 导出（`cliproxy_plugin_init` 等） |
| `plugin.go` | 方法分发、插件注册信息、`request.normalize` |
| `management.go` | Web 界面资源、status / preview 接口 |
| `internal/config` | 配置解析、默认值、校验、模型匹配 |
| `internal/rewrite` | 时区 / 日期改写与注入逻辑 |
| `internal/ui/index.html` | 内嵌的配置页面（原生 HTML/CSS/JS，无外部依赖） |
| `scripts/e2e` | 模拟上游、测试配置与端到端测试脚本 |
