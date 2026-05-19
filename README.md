# Noco Path Opener

Noco Path Opener 是一个运行在 Windows 桌面会话中的小型 Go HTTP 服务。它用于接收 NocoDB 触发的本机路径请求，并在 Windows 主机上打开已有文件/目录，或通过 GUI 选择文件/目录后把绝对路径写回 NocoDB 记录字段。

当前版本：`0.2.0`

典型场景：NocoDB 运行在 Docker 容器中，表格按钮或 webhook 把记录信息发送给 Windows 主机上的本服务；本服务弹出一个本机 GUI，让用户选择“打开当前路径”或“上传/更新本地路径”。

## 功能

- `POST /open`：直接打开请求中的本机文件或目录。
- `POST /webhook`：接收 NocoDB 行记录信息，打开本机 GUI 操作窗口。
- GUI 支持打开记录中的 `current_path`。
- GUI 支持拖放或选择单个文件/目录，并将其绝对路径更新回 NocoDB 文本字段。
- GUI 更新成功后返回初始操作界面，便于继续打开或更新。
- GUI 更新成功后，本次窗口的 `打开` 按钮会使用刚写回的新路径。
- GUI 创建时会靠近鼠标位置，并尝试置于网页/其他窗口前方。
- Windows 版本默认不显示命令行窗口，程序常驻系统托盘，可通过托盘菜单退出。
- `max_gui_windows` 可控制同时弹出的 GUI 窗口数量，默认 `1`，超出后排队。
- `allowed_roots` 可限制允许打开或写回的路径范围。
- `nocodb_url` 和 `nocodb_token` 仅保存在本机 `config.json` 中，不需要放进 webhook payload。

## 工作流

`/webhook` 的完整流程：

1. NocoDB 发送记录信息到本服务。
2. 本服务立即返回 `202 Accepted`，表示请求已进入处理流程。
3. 本服务按 `max_gui_windows` 限制打开 GUI；超过上限的请求等待前面的 GUI 结束。
4. 用户在 GUI 中点击 `打开`，程序打开 `current_path` 指向的本机路径。
5. 用户在 GUI 中点击 `上传或更新`，选择或拖放文件/目录。
6. 程序校验路径存在性和 `allowed_roots`。
7. 用户确认后，程序调用 NocoDB v3 Data API，把指定字段更新为本机绝对路径。

`/open` 是兼容接口：它只打开请求体里的路径，不打开 GUI，也不会写回 NocoDB。

## 快速开始

### 1. 构建 Windows 可执行文件

在 WSL 或 Linux 环境中运行：

```bash
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o noco-path-opener.exe ./cmd/noco-path-opener
```

如果 Go 不在 `PATH` 中，可以使用 Go 可执行文件的绝对路径：

```bash
GOOS=windows GOARCH=amd64 /home/ccamj/go/bin/go build -ldflags="-H windowsgui" -o noco-path-opener.exe ./cmd/noco-path-opener
```

仓库已在 `cmd/noco-path-opener/rsrc_windows_amd64.syso` 内嵌 Windows Common Controls 6 manifest。使用上面的 `windows/amd64` 构建命令时，GUI 运行不需要额外复制 `.manifest` 文件。

### 2. 放到 Windows 主机

把 `noco-path-opener.exe` 放到 Windows 上固定目录，例如桌面或工具目录。`config.json` 会放在 exe 同目录，因此建议选择一个普通用户有写权限、且不容易被误删的位置。

### 3. 首次启动并生成配置

双击 `noco-path-opener.exe`。程序启动后不会显示命令行窗口，会在系统托盘显示图标；需要退出时，右键托盘图标选择 `退出`。首次启动时，会在 exe 同目录生成默认 `config.json`：

```json
{
  "host": "0.0.0.0",
  "port": 6666,
  "allowed_roots": [],
  "max_gui_windows": 1,
  "nocodb_url": "http://localhost:8080",
  "nocodb_token": ""
}
```

如果使用不带 `-H windowsgui` 的调试构建运行，控制台会输出类似：

```text
noco-path-opener listening on http://0.0.0.0:6666/open and http://0.0.0.0:6666/webhook
```

### 4. 配置 NocoDB 地址和 token

如果需要用 GUI 把路径写回 NocoDB，必须在 `config.json` 中配置：

```json
{
  "nocodb_url": "http://localhost:8080",
  "nocodb_token": "你的 NocoDB API token"
}
```

只使用 `/open` 或只在 GUI 中点击 `打开` 时，不需要 NocoDB token。点击 `确认更新` 时才会使用 `nocodb_url` 和 `nocodb_token`。

## 配置

`config.json` 必须放在 `noco-path-opener.exe` 同目录。

完整示例：

```json
{
  "host": "0.0.0.0",
  "port": 6666,
  "allowed_roots": [
    "C:\\Users\\YourName\\Documents",
    "D:\\Work"
  ],
  "max_gui_windows": 1,
  "nocodb_url": "http://localhost:8080",
  "nocodb_token": ""
}
```

字段说明：

| 字段 | 默认值 | 校验规则 | 说明 |
| --- | --- | --- | --- |
| `host` | `0.0.0.0` | 不能为空 | HTTP 监听地址。Docker 容器访问 Windows 主机时通常需要监听 `0.0.0.0`。 |
| `port` | `6666` | `1` 到 `65535` | HTTP 监听端口。 |
| `allowed_roots` | `[]` | 必须是数组；元素不能为空字符串 | 允许打开或更新的路径根目录。空数组表示不限制路径。建议填写 Windows 绝对路径。 |
| `max_gui_windows` | `1` | 省略或填 `0` 时按 `1` 处理；小于 `0` 无效 | 同时允许弹出的 GUI 窗口数量。超过上限的 `/webhook` 请求会等待。 |
| `nocodb_url` | `http://localhost:8080` | 可为空 | NocoDB 实例地址。执行路径更新时必须配置。 |
| `nocodb_token` | 空字符串 | 可为空 | NocoDB API token。执行路径更新时必须配置。 |

注意：

- 首次生成的 `config.json` 会包含全部默认字段。
- 已存在的旧 `config.json` 不会被程序自动重写；如果缺少 `max_gui_windows`，程序运行时会按 `1` 处理。需要文件中显式显示该字段时，请手动添加。
- 修改 `config.json` 后需要重启程序才会生效。
- `allowed_roots` 为空时不做路径限制，请只在可信环境中使用。

## NocoDB Webhook 配置

如果 NocoDB 运行在 Docker Desktop 容器中，通常从容器访问 Windows 主机地址：

```text
http://host.docker.internal:6666/webhook
```

请求方法和内容类型：

```text
POST http://host.docker.internal:6666/webhook
Content-Type: application/json
```

NocoDB 自定义 payload 示例（这是模板 payload，不是直接发送的原始 JSON）：

```text
{
  "base_id": "p_xxxxx",
  "table_id": {{ json event.data.table_id }},
  "record_id": {{ json event.data.rows.[0].Id }},
  "path_field": "本地文件路径",
  "current_path": {{ json event.data.rows.[0].本地文件路径 }}
}
```

字段含义：

| 字段 | 是否必填 | 类型 | 说明 |
| --- | --- | --- | --- |
| `base_id` | 是 | string | NocoDB base/project ID，例如 `p_xxxxx`。 |
| `table_id` | 是 | string | NocoDB table ID，例如 `m_xxxxx`。 |
| `record_id` | 是 | string 或 number | 要更新的记录 ID。空字符串、`null`、对象都无效。 |
| `path_field` | 是 | string | 要写回路径的 NocoDB 文本字段名。 |
| `current_path` | 否 | string | 当前记录中已有的本机路径。GUI 的 `打开` 按钮会使用它。 |

成功响应：

```json
{
  "success": true,
  "queued": true
}
```

状态码是 `202 Accepted`。这只表示请求已被服务接收并进入 GUI 处理流程，不代表用户已经打开文件或更新 NocoDB 成功。

## API

### `POST /open`

直接打开本机路径，不弹出 GUI，不更新 NocoDB。

请求体：

```json
{
  "path": "D:\\docs\\a.docx"
}
```

请求字段：

| 字段 | 是否必填 | 类型 | 说明 |
| --- | --- | --- | --- |
| `path` | 是 | string | 要打开的本机文件或目录路径。建议使用 Windows 绝对路径。 |

文件成功响应：

```json
{
  "success": true,
  "path": "D:\\docs\\a.docx",
  "type": "file"
}
```

目录成功响应里的 `type` 是 `directory`。

错误响应：

```json
{
  "success": false,
  "error": "path does not exist"
}
```

常见状态码：

| 状态码 | 场景 |
| --- | --- |
| `200` | 路径存在，且 Windows 打开命令已成功发出 |
| `400` | JSON 无效，或 `path` 缺失/为空 |
| `403` | 路径不在 `allowed_roots` 允许范围内 |
| `404` | 路径不存在，或路由不存在 |
| `405` | 请求方法不是 `POST` |
| `500` | 路径存在，但 Windows 打开失败 |

### `POST /webhook`

打开本机 GUI 操作窗口。HTTP 响应只表示请求已进入队列，不代表 GUI 后续操作成功。

请求体：

```json
{
  "base_id": "p_xxxxx",
  "table_id": "m_xxxxx",
  "record_id": 123,
  "path_field": "本地文件路径",
  "current_path": "C:\\Users\\YourName\\Desktop\\old.docx"
}
```

成功响应：

```json
{
  "success": true,
  "queued": true
}
```

常见状态码：

| 状态码 | 场景 |
| --- | --- |
| `202` | 请求有效，已进入 GUI 处理流程 |
| `400` | JSON 无效，存在尾随内容，或缺少 `base_id`、`table_id`、`record_id`、`path_field` |
| `405` | 请求方法不是 `POST` |
| `500` | webhook dispatcher 未配置 |

## Webhook GUI 行为

`/webhook` 接收请求后会打开标题为 `文件操作` 的窗口。窗口包含 `打开`、`上传或更新`、`取消`。

- 窗口初始尺寸较小，仅展示提示、按钮和状态文本。
- 窗口创建位置靠近鼠标，并会尝试置前；最终前台焦点仍受 Windows 系统策略影响。
- 多次 `/webhook` 请求会按 `max_gui_windows` 控制同时弹出的窗口数量，默认一次只显示 1 个，其余请求等待。
- `打开` 会读取 payload 中的 `current_path`，检查是否为空、是否在 `allowed_roots` 内、是否存在，然后用 Windows 默认方式打开文件或目录。
- `打开` 成功后窗口会短暂显示 `已打开。`，随后自动关闭。
- `上传或更新` 会打开一个路径选择器，支持选择单个文件或目录。
- 也可以直接拖放文件或目录到 GUI 窗口中；如果一次拖放多个路径，只处理第一个。
- 第一次选择路径时，选择器默认从当前用户主目录开始；重新选择时，会优先从上一次已选路径所在目录开始。
- 选择或拖放的路径会转换成绝对路径，并检查 `allowed_roots` 和本地存在性。
- 路径通过校验后，GUI 进入确认页面，展示 `path_field` 和将要写回的路径。
- 点击 `确认更新` 后，程序调用 NocoDB v3 Data API，把 `path_field` 指定字段更新为本机绝对路径。
- 更新成功后，同一个窗口里再次点击 `打开` 会使用刚更新的新路径。
- 更新成功后，GUI 返回初始 `打开` / `上传或更新` 界面，不自动关闭。
- 打开或更新失败时，错误会显示在 GUI 状态文本中，窗口不会自动关闭，用户可以重试或取消。
- 正在打开或更新时，按钮会暂时禁用；此时关闭窗口会被阻止，避免中断正在进行的操作。

## 手动验证

在 Windows 上启动程序后，可以从 NocoDB 容器、WSL 或其他终端发送请求。

验证 `/open` 打开文件：

```bash
curl -i -X POST http://host.docker.internal:6666/open \
  -H 'Content-Type: application/json' \
  -d '{"path":"C:\\Users\\YourName\\Desktop\\test.txt"}'
```

验证 `/open` 打开目录：

```bash
curl -i -X POST http://host.docker.internal:6666/open \
  -H 'Content-Type: application/json' \
  -d '{"path":"C:\\Users\\YourName\\Documents"}'
```

验证 `/open` 参数错误：

```bash
curl -i -X POST http://host.docker.internal:6666/open \
  -H 'Content-Type: application/json' \
  -d '{"path":"   "}'
```

验证 `/webhook` 打开 GUI：

```bash
curl -i -X POST http://host.docker.internal:6666/webhook \
  -H 'Content-Type: application/json' \
  -d '{"base_id":"p_xxxxx","table_id":"m_xxxxx","record_id":123,"path_field":"本地文件路径","current_path":"C:\\Users\\YourName\\Desktop\\test.txt"}'
```

验证不存在路径：

```bash
curl -i -X POST http://host.docker.internal:6666/open \
  -H 'Content-Type: application/json' \
  -d '{"path":"C:\\path\\that\\does\\not\\exist.txt"}'
```

## 安全提醒

默认配置监听 `0.0.0.0:6666`，并且没有 token 认证。这是为了让 Docker 里的 NocoDB 能访问 Windows 主机，但也可能让同一网络里的其他设备访问该接口，具体取决于 Windows 防火墙规则。

建议：

- 只在可信网络中运行。
- 用 Windows 防火墙限制访问来源。
- 使用 `allowed_roots` 限制可打开和可写回的目录。
- 保护 `config.json` 文件权限；`nocodb_token` 以明文保存在本机配置中。
- 不要把端口暴露到公网。
- 不要把 `nocodb_token` 放入 NocoDB webhook payload 或日志中。

## 故障排查

| 现象 | 检查项 |
| --- | --- |
| NocoDB 容器访问不到服务 | 确认程序正在 Windows 主机运行；Docker Desktop 通常使用 `host.docker.internal`；检查 Windows 防火墙和监听端口。 |
| `/open` 返回 `path not allowed` | 检查 `allowed_roots` 是否包含目标路径的父目录；建议使用 Windows 绝对路径并注意盘符。 |
| `/open` 返回 `path does not exist` | 确认路径在 Windows 主机上存在；容器内部路径不能直接作为 Windows 本机路径使用。 |
| GUI 点击 `打开` 后提示当前路径为空 | NocoDB payload 没有传 `current_path`，或字段模板解析为空。 |
| GUI 点击 `确认更新` 后提示需要 NocoDB 配置 | 检查 `nocodb_url` 和 `nocodb_token` 是否已填写并重启程序。 |
| 多次请求后 GUI 没有马上全部出现 | 这是 `max_gui_windows` 的限制行为；默认只同时显示 1 个窗口，其余请求等待。 |
| GUI 没有出现在最前面 | 程序会尝试置前，但 Windows 可能限制后台进程抢焦点；检查任务栏或其他显示器。 |
| Linux/WSL 上不能看到 GUI | GUI 只支持 Windows 桌面会话；Linux/WSL 主要用于构建和自动化测试。 |

## 开发

要求：Go 1.22 或更高版本。

运行测试：

```bash
go test ./...
```

构建当前平台版本：

```bash
go build ./cmd/noco-path-opener
```

构建 Windows 版本：

```bash
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o noco-path-opener.exe ./cmd/noco-path-opener
```

发布版构建（可选）：

```bash
mkdir -p dist
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H windowsgui" -o dist/noco-path-opener.exe ./cmd/noco-path-opener
```

项目结构：

```text
cmd/noco-path-opener/      程序入口
internal/actions/          webhook GUI 动作编排和并发限制
internal/config/           配置读取、默认配置生成、配置校验
internal/gui/              Windows GUI 和非 Windows stub
internal/nocodb/           NocoDB v3 Data API 客户端
internal/openapi/          HTTP 路由、请求校验、JSON 响应
internal/pathauth/         allowed_roots 路径授权判断
internal/tray/             Windows 托盘图标和退出菜单
internal/winopen/          Windows 打开文件/目录的边界实现
```

## 已知限制

- 不是 Windows 服务，需要在用户桌面会话中手动启动。
- HTTP 接口没有 token 认证。
- 不限制文件扩展名。
- 不会自动迁移或重写已有 `config.json`。
- Linux/WSL 上只能验证 HTTP、JSON、配置和源码级行为，不能真的打开 Windows 桌面 GUI。
