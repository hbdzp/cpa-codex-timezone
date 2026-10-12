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
- [通过插件商店安装（信任第三方源）](#通过插件商店安装信任第三方源)
- [发布到插件商店](#发布到插件商店)
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

- 只改写 `<environment_context>` 块内的标签，范围是 `instructions` 和 `input[]` 中所有非 assistant 的消息，`<timezone status="unavailable" />` 这种写法也会被替换。用户在提问或粘贴的代码里写到的同名标签不会被改动。
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
| `make package` | `dist/codex-timezone_<版本>_<系统>_<架构>.zip` 和 `dist/checksums.txt` | 插件商店使用的发布包，见[发布到插件商店](#发布到插件商店) |
| `make registry BASE_URL=...` | `dist/registry.json` | 自建插件商店的索引文件 |

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

## 通过插件商店安装（信任第三方源）

除了手动复制文件，CPA v8 还内置了插件商店。CPA 默认只信任官方源 [CLIProxyAPI-Plugins-Store](https://github.com/router-for-me/CLIProxyAPI-Plugins-Store)。把发布方提供的 `registry.json` 地址加入 `plugins.store-sources` 后，就可以从这个**第三方源**一键安装和升级插件。

商店安装会自动完成以下几件事：

1. 下载与当前 CPA 系统和架构匹配的发布包；
2. 校验 SHA256；
3. 把插件放到 `plugins/<系统>/<架构>/codex-timezone-v<版本>.so`；
4. 在 `config.yaml` 中写入 `plugins.configs.codex-timezone.enabled: true` 和来源信息（`store` 段）；
5. 热加载插件，不需要重启 CPA。

> **安全提示：** 插件是原生动态库，会在 CPA 进程内以 CPA 的全部权限运行。“信任一个源”意味着信任这个源发布的任意代码。SHA256 校验只能保证下载的文件与 `registry.json` 中登记的一致，不能证明发布者可信。请只添加你信任的源，并尽量使用 HTTPS 地址。

### 第 1 步：准备 CPA

`config.yaml` 中至少需要：

```yaml
management:
  secret-key: "你的管理密钥"   # 商店接口属于管理接口，必须设置
plugins:
  enabled: true
  dir: "plugins"
```

### 第 2 步：信任第三方源

把源地址加入 `plugins.store-sources`，可以写多个。官方源始终保留，不用写：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  store-sources:
    - "https://example.com/cpa-plugins/registry.json"
```

不想手动编辑文件时，也可以调用管理接口。注意 `PUT` 会**覆盖**整个列表，已有的源要一并写上：

```bash
curl -X PUT http://127.0.0.1:8317/v8/management/config/plugins/store-sources \
  -H "Authorization: Bearer <管理密钥>" -H "Content-Type: application/json" \
  -d '["https://example.com/cpa-plugins/registry.json"]'
```

**源地址是 HTTP（非 HTTPS）时**，例如内网服务器，CPA 默认会拒绝，报错 `insecure plugin store url requires matching allow-insecure auth rule`。需要再加一条匹配该地址前缀的规则，显式允许：

```yaml
plugins:
  store-auth:
    - match: "http://192.168.1.10:8000/"   # 地址前缀，需要同时覆盖 registry.json 和发布包的地址
      apply-to: ["registry", "artifact"]
      type: "none"
      allow-insecure: true
```

**源需要鉴权时**（私有源），在同一处配置鉴权方式。凭据从环境变量读取，不会写入配置文件：

```yaml
plugins:
  store-auth:
    - match: "https://plugins.example.com/"
      apply-to: ["registry", "artifact"]
      type: "bearer"                        # 也支持 basic（username-env / password-env）和 header（header-name / header-value-env）
      token-env: "CLIPROXY_PLUGIN_STORE_TOKEN"
```

### 第 3 步：安装

先查看商店列表，找到本插件以及它所在源的 `source_id`：

```bash
curl -s http://127.0.0.1:8317/v8/management/plugins/store -H "Authorization: Bearer <管理密钥>"
```

返回结果中：

- `sources` 列出了所有已信任的源；
- `source_errors` 列出了读取失败的源及原因；
- `plugins` 中 `id` 为 `codex-timezone` 的条目就是本插件，例如：

```json
{
  "store_id": "source-6fa0c4672b02/codex-timezone",
  "source_id": "source-6fa0c4672b02",
  "id": "codex-timezone",
  "name": "Codex 时区",
  "version": "0.1.0",
  "install_type": "direct",
  "platforms": [{"goos": "linux", "goarch": "amd64"}],
  "installed": false
}
```

`source_id` 由源地址计算得到，同一个地址的 `source_id` 始终不变。然后安装：

```bash
curl -X POST "http://127.0.0.1:8317/v8/management/plugins/store/codex-timezone/install?source=<source_id>" \
  -H "Authorization: Bearer <管理密钥>"
# 安装指定版本：在 URL 后加 &version=0.1.0
```

返回 `"status":"installed"` 和安装路径，并且 `"restart_required":false`，就说明插件已经加载。如果 CPA 自带的管理面板有插件商店页面，也可以在页面上完成同样的操作，页面调用的是同一组接口。

### 第 4 步：设置时区

商店安装只会写入 `enabled: true`，此时插件使用默认配置，时区为 `Asia/Shanghai`。打开 [Web 配置界面](#使用-web-配置界面)，按需修改并保存即可。

### 升级、切换源与卸载

- **升级**：源发布新版本后，商店列表中的 `update_available` 会变为 `true`。再调用一次上面的安装接口即可，CPA 会热切换到新版本，已有的时区设置保持不变。
- **切换源**：从商店安装的插件会记住来源。改从另一个源安装会报 `plugin_store_source_conflict`，需要先卸载。
- **从手动安装改为商店安装**：先删掉手动放入的 `codex-timezone*.so`，避免插件目录里出现两个同 ID 的文件。
- **卸载**：

```bash
curl -X DELETE http://127.0.0.1:8317/v8/management/plugins/codex-timezone -H "Authorization: Bearer <管理密钥>"
```

  卸载会同时删除插件文件和 `plugins.configs.codex-timezone` 配置，包括时区设置。如果返回 `plugin_delete_requires_restart`，重启 CPA 后再执行一次。

---

## 发布到插件商店

这一节写给插件的**发布者**，介绍如何让其他人通过商店安装本插件。CPA 支持两种发布方式：

| 方式 | 发布包放在哪里 | registry.json | 适合 |
| --- | --- | --- | --- |
| GitHub Release | GitHub 仓库的 Release | 只登记仓库地址，`schema_version: 1` | 代码托管在 GitHub，或者想提交到官方商店 |
| 直链下载（direct） | 任意 HTTP(S) 服务器，例如对象存储、GitLab Release、内网服务器 | 登记每个平台的下载地址和 SHA256，`schema_version: 2` | 自建源、内网部署、非 GitHub 托管 |

两种方式使用相同的发布包格式：

- 文件名：`codex-timezone_<版本>_<系统>_<架构>.zip`，例如 `codex-timezone_0.1.0_linux_amd64.zip`；
- zip **根目录**放 `codex-timezone.so` 或 `codex-timezone-v0.1.0.so`（macOS 为 `.dylib`，Windows 为 `.dll`），不要套子目录。

`make package` 会按这个格式打包当前平台的插件，并生成 `checksums.txt`：

```bash
make package VERSION=0.1.0 REPOSITORY=https://github.com/you/cpa-codex-timezone
# dist/codex-timezone_0.1.0_linux_amd64.zip
# dist/checksums.txt
```

需要支持多个平台时，在每个平台（或对应的 Docker 环境）上分别执行 `make package`，把生成的 zip 都汇总到同一个 `dist/` 目录，然后在该目录下重新执行一次 `sha256sum *.zip > checksums.txt`。

### 方式一：GitHub Release

1. 在 GitHub 仓库创建 Release，**tag 为 `v0.1.0` 或 `0.1.0`**，CPA 会从 tag 中解析出版本号。
2. 上传附件：所有平台的 `codex-timezone_0.1.0_<系统>_<架构>.zip`，以及 **`checksums.txt`**（必须有，CPA 安装时会用它校验）。
3. 在 registry.json 中登记插件。CPA 默认安装最新的 Release，也可以通过 `version` 参数安装指定版本：

```json
{
  "schema_version": 1,
  "plugins": [
    {
      "id": "codex-timezone",
      "name": "Codex 时区",
      "description": "在 CPA 把请求发往 Codex / GPT 模型前，改写请求中的时区与当前日期，并提供中文 Web 配置界面。",
      "author": "你的名字",
      "repository": "https://github.com/you/cpa-codex-timezone",
      "homepage": "https://github.com/you/cpa-codex-timezone",
      "tags": ["Codex", "Timezone"]
    }
  ]
}
```

registry.json 可以放在任意可访问的地址（例如仓库里的 raw 文件地址），供使用者加入 `store-sources`。也可以向 [官方商店仓库](https://github.com/router-for-me/CLIProxyAPI-Plugins-Store) 提交 PR，把上面的条目加入官方 `registry.json`，这样使用者不需要添加第三方源。

> 匿名访问 GitHub API 有频率限制。使用者遇到 `plugin_store_rate_limited` 时，可以为 `https://api.github.com/` 配置一条 `type: "bearer"` 的 `store-auth` 规则，使用 GitHub token。

### 方式二：直链下载（自建源）

1. 把 `dist/` 中的 zip 上传到一个可以直接下载的地址，例如 `https://example.com/cpa-plugins/codex-timezone/0.1.0/`。
2. 生成 registry.json：

```bash
make registry VERSION=0.1.0 BASE_URL=https://example.com/cpa-plugins/codex-timezone/0.1.0
# 等价于：
python3 scripts/store/gen_registry.py --dist dist --version 0.1.0 \
  --base-url https://example.com/cpa-plugins/codex-timezone/0.1.0 --out dist/registry.json
```

   脚本会为 `dist/` 中每个平台的 zip 写入下载地址、SHA256 和文件大小。生成的条目大致如下：

```json
{
  "schema_version": 2,
  "plugins": [{
    "id": "codex-timezone",
    "name": "Codex 时区",
    "description": "…",
    "author": "cpa-codex-timezone",
    "version": "0.1.0",
    "install": {
      "type": "direct",
      "artifacts": [{
        "goos": "linux", "goarch": "amd64",
        "url": "https://example.com/cpa-plugins/codex-timezone/0.1.0/codex-timezone_0.1.0_linux_amd64.zip",
        "sha256": "191ccd81…", "size": 2215259
      }]
    }
  }]
}
```

3. 把 `registry.json` 上传到一个固定地址，例如 `https://example.com/cpa-plugins/registry.json`，告诉使用者把这个地址加入 `store-sources`。

**发布新版本**：用 `--merge` 更新已有的 registry.json。旧版本会自动移到 `versions` 列表中，仍然可以通过 `&version=` 安装；registry.json 中其他插件的条目保持不变：

```bash
make package VERSION=0.2.0
python3 scripts/store/gen_registry.py --dist dist --version 0.2.0 \
  --base-url https://example.com/cpa-plugins/codex-timezone/0.2.0 \
  --merge registry.json --out registry.json
```

注意事项：

- 下载地址的查询参数中不能包含 `token`、`api_key`、`secret` 等敏感字段，CPA 会拒绝这样的地址。私有下载请改用上文的 `store-auth` 鉴权规则。
- 发布后不要替换同名 zip 的内容，否则 SHA256 对不上，安装会失败。发布新内容请使用新版本号。

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

> 通过插件商店安装的，升级和卸载直接调用商店接口即可，见[升级、切换源与卸载](#升级切换源与卸载)。

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
| Web 界面能打开，但连接时提示“没有找到管理接口” | `management.secret-key` 为空时，CPA 会关闭全部管理接口（返回 404），设置密钥后重启 CPA |
| Web 界面提示“管理密钥无效” | 密钥不对；`management.secret-key` 为空；或者从远程访问但 `allow-remote` 为 `false` |
| 保存后一直提示“等待生效” | 配置文件已写入，但 CPA 拒绝了新配置（例如时区名无效），查看 CPA 日志中的 `plugin.reconfigure` 警告 |
| `matched` 一直为 0 | 模型名不匹配 `models` 规则，例如带了前缀 `team/gpt-5.5`，可以把规则改成 `*gpt-*`；或者请求没有发往 Codex 上游 |
| `matched` 增加但 `rewritten` 不增加 | 请求中没有时区标签，并且 `inject` 为 `off` |
| 模型回答的时间仍然不对 | 查看状态接口中的生效时区；也可以用 Web 界面的“改写预览”粘贴真实请求体，检查改写结果 |
| 商店列表里看不到本插件 | 先看返回结果中的 `source_errors`：源地址无法访问、JSON 格式错误，或者缺少 `id` / `name` / `description` / `author` / `repository` 等必填字段 |
| 商店报 `insecure plugin store url requires matching allow-insecure auth rule` | 源地址或下载地址是 `http://`，需要在 `store-auth` 中为它加一条 `allow-insecure: true` 的规则，且 `apply-to` 同时包含 `registry` 和 `artifact` |
| 安装报 `plugin_store_source_conflict` | 插件已经从另一个源安装过，先卸载再从新源安装 |
| 安装报 `release asset ... not found` / `release asset checksums.txt not found` | GitHub Release 中缺少当前平台的 zip（`codex-timezone_<版本>_<系统>_<架构>.zip`）或 `checksums.txt`，用 `make package` 生成后一起上传 |
| 安装报 `checksum mismatch` / `sha256 mismatch` | 下载到的文件与源中登记的 `sha256` / `size` 不一致，重新执行 `make registry` 生成源文件 |
| 安装报 `plugin_store_rate_limited` | GitHub API 限流，在 `store-auth` 中为 `https://api.github.com/` 配置 bearer token |
| 升级或卸载报 `plugin_update_requires_restart` / `plugin_delete_requires_restart` | 插件正在处理请求，无法热替换，重启 CPA 后再执行一次 |

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

插件商店的端到端测试会打包两个版本、生成直链源、用本地 HTTP 服务托管，然后在一个没有预装插件的 CPA 上依次验证：信任第三方源、列表、安装、发现更新、热升级。

```bash
CPA_BIN=$PWD/CLIProxyAPI/cli-proxy-api make e2e-store
```

这个测试的 CPA 监听 `127.0.0.1:48318`，商店源托管在 `127.0.0.1:47400`，产物和日志保留在 `.e2e-store/` 目录。

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
| `scripts/e2e` | 模拟上游、测试配置与端到端测试脚本（含商店测试 `store.sh`） |
| `scripts/store` | 插件商店源文件生成脚本 `gen_registry.py` |
