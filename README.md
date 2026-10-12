# cpa-codex-timezone

[![CI](https://github.com/hbdzp/cpa-codex-timezone/actions/workflows/ci.yml/badge.svg)](https://github.com/hbdzp/cpa-codex-timezone/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/hbdzp/cpa-codex-timezone)](https://github.com/hbdzp/cpa-codex-timezone/releases/latest)

[CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)（CPA）插件：在请求发往 Codex / GPT 模型前，把其中的**时区**和**当前日期**改成你指定的时区，并提供中文 Web 配置界面。

CPA 部署在海外服务器、或客户端时区不对时，模型会按错误的时区理解“今天”和“现在几点”。装上插件后，模型看到的始终是你设置的时区。

- 适用于 CPA v8（v8.0.26 验证通过）
- 只处理发往 Codex 上游、且模型名匹配规则（默认 `gpt-*`）的请求，其他请求原样转发
- 在 Web 界面修改配置，保存后热生效，无需重启

## 效果

Codex CLI 每次请求都会带上 `<environment_context>`，插件按目标时区改写其中的时区和日期：

```diff
 <environment_context>
   <cwd>/home/me/project</cwd>
-  <current_date>2026-10-12</current_date>
-  <timezone>America/Los_Angeles</timezone>
+  <current_date>2026-10-13</current_date>
+  <timezone>Asia/Shanghai</timezone>
 </environment_context>
```

OpenAI SDK、`/v1/chat/completions` 等请求没有这段内容，插件会在 `input` 末尾追加一条 developer 消息（追加在末尾，不影响上游提示缓存）：

```text
Current timezone: Asia/Shanghai. Current local time: 2026-10-13 00:10:34 +08:00.
```

只改写 `<environment_context>` 块内的标签；用户提问或粘贴的代码里出现的同名标签不会被改动。

## 安装

支持 Linux amd64 / arm64、macOS amd64 / arm64、Windows amd64。Linux 需要 glibc 2.34+（Debian 12、Ubuntu 22.04+、CPA 官方 Docker 镜像均满足，不支持 Alpine）。

CPA 的 `config.yaml` 需要先开启插件和管理接口：

```yaml
management:
  secret-key: "你的管理密钥"   # Web 界面和商店安装都需要；留空会关闭管理接口

plugins:
  enabled: true
  dir: "plugins"              # 相对 CPA 的工作目录
```

### 方式一：插件商店（推荐）

在 `plugins` 下加入本仓库的源：

```yaml
plugins:
  store-sources:
    - "https://raw.githubusercontent.com/hbdzp/cpa-codex-timezone/main/registry.json"
```

重启 CPA 后安装（`source-a4229a9ecaa6` 是这个源地址对应的固定 ID）：

```bash
curl -X POST "http://127.0.0.1:8317/v8/management/plugins/store/codex-timezone/install?source=source-a4229a9ecaa6" \
  -H "Authorization: Bearer <管理密钥>"
```

CPA 会下载最新 Release 中对应平台的包、校验 SHA256 并热加载，同时在配置中写入 `enabled: true`。如果 CPA 管理面板有插件商店页面，也可以在页面上安装。

> 插件是原生动态库，以 CPA 进程的权限运行。只添加你信任的源。

### 方式二：下载预编译包

从 [Releases](https://github.com/hbdzp/cpa-codex-timezone/releases/latest) 下载对应平台的 zip，解压到插件目录：

```bash
VERSION=0.1.0
curl -LO https://github.com/hbdzp/cpa-codex-timezone/releases/download/v$VERSION/codex-timezone_${VERSION}_linux_amd64.zip
unzip -o codex-timezone_${VERSION}_linux_amd64.zip -d /opt/cliproxyapi/plugins/
```

然后在 `config.yaml` 中启用插件并重启 CPA：

```yaml
plugins:
  configs:
    codex-timezone:
      enabled: true
      timezone: "Asia/Shanghai"
```

Docker 部署时，官方 `docker-compose.yml` 已把宿主机的 `./plugins` 挂载到容器内，把 Linux 包解压到该目录即可。

### 方式三：自己编译

需要 Go 1.26+ 和 C 编译器（cgo），且必须在与 CPA 相同的系统和架构上编译：

```bash
git clone https://github.com/hbdzp/cpa-codex-timezone.git
cd cpa-codex-timezone
make dist        # 产物：bin/codex-timezone-v0.1.0.so（macOS 为 .dylib，Windows 为 .dll）
```

把产物复制到插件目录，按方式二启用即可。

> 插件文件名必须是 `codex-timezone.<扩展名>` 或 `codex-timezone-v<版本>.<扩展名>`，CPA 由文件名确定插件 ID。

## 配置

最简单的方式是使用 [Web 界面](#web-配置界面)。也可以直接写在 `config.yaml` 中，修改后 CPA 会自动热重载：

```yaml
plugins:
  configs:
    codex-timezone:
      enabled: true
      timezone: "Asia/Shanghai"
      models: ["gpt-*"]
      rewrite_current_date: true
      inject: "if_missing"
      inject_template: "Current timezone: {tz}. Current local time: {now}."
```

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `enabled` | `false` | 是否启用 |
| `timezone` | `Asia/Shanghai` | IANA 时区名，如 `Asia/Tokyo`、`Europe/London`、`UTC` |
| `models` | `["gpt-*"]` | 要处理的模型，`*` 为通配符，不区分大小写 |
| `rewrite_current_date` | `true` | 是否按目标时区改写 `<current_date>` |
| `inject` | `if_missing` | 追加时间说明：`off` 从不、`if_missing` 请求中没有 `<timezone>` 时、`always` 总是 |
| `inject_template` | 见上 | 说明模板，占位符：`{tz}` `{now}` `{date}` `{time}` `{offset}` `{weekday}` |

配置无效（如时区名拼错）时，CPA 会拒绝新配置，插件继续使用上一份有效配置。插件内置时区数据库，不依赖系统 zoneinfo。

## Web 配置界面

```text
http://<CPA地址>:8317/v0/resource/plugins/codex-timezone/ui
```

输入管理密钥连接后即可：

- 从 80 多个预设城市中选择时区，或输入 IANA 名称、中文城市名（如“东京”），实时显示该时区的当前时间；
- 管理匹配的模型、注入方式和模板；
- 用“改写预览”粘贴请求体，对比改写前后的内容（使用未保存的设置，不发出真实请求）；
- 点击“保存并生效”，配置写入 `config.yaml` 并热生效；右侧显示运行状态和请求计数。

从其他机器访问时，需要在 CPA 中设置 `management.allow-remote: true`。

## 确认已生效

CPA 日志中出现 `plugin loaded plugin_id=codex-timezone` 即加载成功。也可以查看状态接口：

```bash
curl -H "Authorization: Bearer <管理密钥>" http://127.0.0.1:8317/v0/management/codex-timezone/status
```

返回生效配置、当地时间和计数：`seen` 经过插件、`matched` 命中模型、`rewritten` 实际改写、`injected` 追加了说明。最直接的验证是问模型“现在几点？今天几号？”。

## 升级与卸载

**商店安装的**：Release 发布新版本后，商店列表中 `update_available` 变为 `true`，再调用一次安装接口即可热升级，设置保留。卸载：

```bash
curl -X DELETE http://127.0.0.1:8317/v8/management/plugins/codex-timezone -H "Authorization: Bearer <管理密钥>"
```

**手动安装的**：放入新版本文件 `codex-timezone-v<新版本>.so` 并删除旧文件，然后在 Web 界面点一次“保存并生效”触发重载，即可热升级。卸载时删除插件文件和 `plugins.configs.codex-timezone` 段，重启 CPA。

临时停用：在 Web 界面关闭“启用插件”，或设置 `enabled: false`。

## 常见问题

| 现象 | 原因与解决 |
| --- | --- |
| 日志里没有 `plugin loaded` | `plugins.enabled` 或插件的 `enabled` 不是 `true`；文件不在 `plugins.dir` 中（相对 CPA 工作目录）；文件名被改过 |
| `GLIBC_2.34 not found` | 系统 glibc 太旧（如 CentOS 7、Debian 11），需在目标机器上自己编译 |
| `wrong ELF class` / `cannot open shared object` | 系统或架构不匹配，换用对应平台的包 |
| Web 界面连接时提示找不到管理接口 | 插件未加载，或 `management.secret-key` 为空 |
| Web 界面提示“管理密钥无效” | 密钥错误，或远程访问但未开启 `allow-remote` |
| 保存后一直“等待生效” | CPA 拒绝了新配置，查看日志中的 `plugin.reconfigure` 警告 |
| `matched` 一直为 0 | 模型名不匹配，例如带前缀 `team/gpt-5.5` 时把规则改为 `*gpt-*`；或请求没有发往 Codex 上游 |
| 商店安装报 `plugin_store_rate_limited` | GitHub API 匿名限流，稍后重试，或在 CPA 的 `plugins.store-auth` 中为 `https://api.github.com/` 配置 token |

## 开发

```bash
make test        # 单元测试
make vet
make dist        # 编译
```

端到端测试使用真实 CPA 和模拟的 Codex 上游：

```bash
git clone --branch v8.0.26 https://github.com/router-for-me/CLIProxyAPI.git
(cd CLIProxyAPI && go build -o cli-proxy-api ./cmd/server)
CPA_BIN=$PWD/CLIProxyAPI/cli-proxy-api make e2e         # 改写、注入、热重载
CPA_BIN=$PWD/CLIProxyAPI/cli-proxy-api make e2e-store   # 商店安装与热升级
```

**发布**：推送版本 tag，GitHub Actions 会编译全部平台并创建 Release，商店用户会自动看到更新：

```bash
git tag v0.2.0 && git push origin v0.2.0
```

| 路径 | 说明 |
| --- | --- |
| `plugin.go` / `main.go` | 插件注册、`request.normalize`、C ABI 导出 |
| `management.go` | Web 界面、status / preview 接口 |
| `internal/rewrite` | 时区、日期改写与注入 |
| `internal/config` | 配置解析与校验 |
| `internal/ui/index.html` | 内嵌的配置页面 |
| `registry.json` | 本仓库的插件商店源 |
| `scripts/` | 打包、端到端测试脚本 |

## 许可证

[MIT](LICENSE)
