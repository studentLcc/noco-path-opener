# Noco Path Opener

Noco Path Opener 是一个运行在 Windows 桌面会话中的小型 Go HTTP 服务。它用于接收 NocoDB 触发的本机路径请求，并在 Windows 主机上打开已有文件/目录，或通过 GUI 选择文件/目录后把绝对路径写回 NocoDB 记录字段。

当前版本：`0.3.0`

典型场景：NocoDB 运行在 Docker 容器中，表格按钮或 webhook 把记录信息发送给 Windows 主机上的本服务；本服务弹出一个本机 GUI，让用户选择“打开当前路径”或“上传/更新本地路径”。

## 功能

- `POST /open`：直接打开请求中的本机文件或目录。
- `POST /webhook`：接收 NocoDB 行记录信息，打开本机 GUI 操作窗口。
- GUI 支持打开记录中的 `current_path`。
- GUI 支持拖放或选择单个文件/目录，并将其绝对路径更新回 NocoDB 文本字段。
- GUI 更新成功后返回初始操作界面，便于继续打开或更新。
- GUI 更新成功后，本次窗口的 `打开` 按钮会使用刚写回的新路径。
- GUI 创建时会靠近鼠标位置，并尝试置于网页/其他窗口前方。
- GUI 窗口标题会包含 `record_id`，便于区分来自哪一行。
- Windows 版本默认不显示命令行窗口，程序常驻系统托盘，可通过托盘菜单退出。
- `max_gui_windows` 可控制同时弹出的 GUI 窗口数量，默认 `1`；不同 row 超出后排队，同一个 row 已有窗口时重复请求会把该窗口拉到前台。
- `allowed_roots` 可限制允许打开或写回的路径范围。
- `nocodb_url` 和 `nocodb_token` 仅保存在本机 `config.json` 中，不需要放进 webhook payload。
- 可选的远端字段同步配置允许 `/webhook` 用 `sync_profile` 选择同步规则，并在 GUI 中显示 `同步远端`。

## 工作流

`/webhook` 的完整流程：

1. NocoDB 发送记录信息到本服务。
2. 本服务立即返回 `202 Accepted`，表示请求已进入处理流程。
3. 本服务先按 `base_id/table_id/record_id` 限制同一个 row 只能有 1 个 GUI 请求；重复 row 不会创建第二个请求，如果已有 GUI 窗口会把该窗口拉到前台。
4. 本服务按 `max_gui_windows` 限制打开 GUI；不同 row 超过上限的请求等待前面的 GUI 结束。
5. 用户在 GUI 中点击 `打开`，程序打开 `current_path` 指向的本机路径。
6. 用户点击 `更新` 时选择一个文件或目录，确认后程序只把所选绝对路径写回 `path_field`，不复制文件。
7. 用户点击 `上传` 时选择或拖放多个文件/目录；程序校验路径存在性和 `allowed_roots`，预先检查全部同名冲突后再复制。
8. 上传目标为已有 `current_path` 目录；若 `current_path` 为空，则目标为 `base_dir\\folder_name`，成功后把目标目录路径写回 `path_field`。

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
  "nocodb_token": "",
  "remote_nocodb_url": "",
  "remote_nocodb_token": "",
  "sync_profiles": []
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

只使用 `/open` 或只在 GUI 中点击 `打开` 时，不需要 NocoDB token。点击 `确认更新` 或 `确认上传` 时才会使用 `nocodb_url` 和 `nocodb_token`。

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
  "nocodb_token": "",
  "remote_nocodb_url": "https://remote-nocodb.example.com",
  "remote_nocodb_token": "REMOTE_TOKEN",
  "sync_profiles": [
    {
      "name": "change-log-main",
      "local_base_id": "p_local",
      "local_table_id": "m_local",
      "local_lookup_field": "变更编号",
      "remote_base_id": "p_remote",
      "remote_table_id": "m_remote",
      "remote_lookup_field": "变更编号",
      "sync_fields": [
        "状态",
        "负责人"
      ]
    }
  ]
}
```

字段说明：

| 字段 | 默认值 | 校验规则 | 说明 |
| --- | --- | --- | --- |
| `host` | `0.0.0.0` | 不能为空 | HTTP 监听地址。Docker 容器访问 Windows 主机时通常需要监听 `0.0.0.0`。 |
| `port` | `6666` | `1` 到 `65535` | HTTP 监听端口。 |
| `allowed_roots` | `[]` | 必须是数组；元素不能为空字符串 | 允许打开或更新的路径根目录。空数组表示不限制路径。建议填写 Windows 绝对路径。 |
| `max_gui_windows` | `1` | 省略或填 `0` 时按 `1` 处理；小于 `0` 无效 | 同时允许弹出的 GUI 窗口数量。不同 row 超过上限的 `/webhook` 请求会等待；同 row 重复请求不会创建第二个 GUI，如果已有窗口会把该窗口拉到前台。 |
| `nocodb_url` | `http://localhost:8080` | 可为空 | NocoDB 实例地址。执行路径更新时必须配置。 |
| `nocodb_token` | 空字符串 | 可为空 | NocoDB API token。执行路径更新时必须配置。 |
| `remote_nocodb_url` | 空字符串 | 配置 `sync_profiles` 时必填 | 远端 NocoDB 实例地址。仅点击 `同步远端` 时使用。 |
| `remote_nocodb_token` | 空字符串 | 配置 `sync_profiles` 时必填 | 远端 NocoDB API token。仅点击 `同步远端` 时使用。 |
| `sync_profiles` | `[]` | 必须是数组；每个 profile 的名称、local/remote base/table/lookup 字段和 `sync_fields` 都不能为空；名称和 `sync_fields` 不能重复 | 远端字段同步规则。webhook 的 `sync_profile` 匹配到这里的 `name` 时，GUI 才显示 `同步远端`。 |

注意：

- 首次生成的 `config.json` 会包含全部默认字段。
- 已存在的旧 `config.json` 不会被程序自动重写；如果缺少 `max_gui_windows`，程序运行时会按 `1` 处理。如果缺少 `remote_nocodb_url`、`remote_nocodb_token` 或 `sync_profiles`，程序按未配置远端同步处理。需要文件中显式显示这些字段时，请手动添加。
- 修改 `config.json` 后需要重启程序才会生效。
- `allowed_roots` 为空时不做路径限制，请只在可信环境中使用。
- `sync_profile` is optional。webhook 未传、传空字符串或传入未知 profile 时，请求仍会返回 `202 Accepted`，GUI 不显示 `同步远端`。
- 当 `sync_profile` 匹配到配置时，点击 `同步远端` 会读取本地 row，用 `local_lookup_field` 的值查询远端表，仅把远端记录中的 `sync_fields` 更新回本地 row。远端未找到匹配记录、找到多条匹配记录、本地查询字段为空或远端记录缺少任一 `sync_fields` 字段时都会停止更新。

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
  "current_path": {{ json event.data.rows.[0].本地文件路径 }},
  "base_dir": "D:\\Projects",
  "folder_name": {{ json event.data.rows.[0].项目编号 }},
  "sync_profile": "change-log-main"
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
| `base_dir` | 上传时条件必填 | string | 当 `current_path` 为空时使用的已有基础目录；程序不会自动创建该基础目录。 |
| `folder_name` | 上传时条件必填 | string | 当 `current_path` 为空时，在 `base_dir` 下创建的子目录名，通常填入 NocoDB 字段值。 |
| `sync_profile` | 否 | string | 远端字段同步 profile 名称。匹配 `config.json` 中的 `sync_profiles[].name` 时，GUI 会显示 `同步远端`。 |

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

打开本机 GUI 操作窗口。HTTP 响应只表示请求已进入处理流程，不代表 GUI 后续操作成功；如果同一个 row 已有窗口或待打开请求，不会创建第二个 GUI，已有窗口会被拉到前台。

请求体：

```json
{
  "base_id": "p_xxxxx",
  "table_id": "m_xxxxx",
  "record_id": 123,
  "path_field": "本地文件路径",
  "current_path": "C:\\Users\\YourName\\Desktop\\old.docx",
  "base_dir": "D:\\Projects",
  "folder_name": "P001",
  "sync_profile": "change-log-main"
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

`/webhook` 接收请求后会打开标题包含 `record_id` 的窗口。窗口包含 `打开`、`更新`、`上传`、`取消`。

- 窗口初始尺寸较小，仅展示提示、按钮和状态文本。
- 窗口创建位置靠近鼠标，并会尝试置前；最终前台焦点仍受 Windows 系统策略影响。
- 多次 `/webhook` 请求会按 `max_gui_windows` 控制同时弹出的窗口数量，默认一次只显示 1 个；不同 row 的多余请求等待，同 row 的重复请求不会创建第二个窗口，已有窗口会被拉到前台。
- `打开` 会读取 payload 中的 `current_path`，检查是否为空、是否在 `allowed_roots` 内、是否存在，然后用 Windows 默认方式打开文件或目录。
- `打开` 成功后窗口会短暂显示 `已打开。`，随后自动关闭。
- `更新` 会打开一个路径选择器，支持选择单个文件或目录；更新成功后字段保存文件路径或目录路径，行为保持不变。
- `上传` 支持一次多选文件；进入确认页后还可以通过 `添加文件` 和 `添加文件夹` 继续加入多个文件和目录，最终混合确认上传。上传始终复制来源，不会剪切原路径。
- 第一次选择路径时，选择器默认从当前用户主目录开始；重新选择时，会优先从上一次已选路径所在目录开始。
- 选择或拖放的路径会转换成绝对路径，并检查 `allowed_roots` 和本地存在性。
- 更新路径通过校验后，GUI 进入确认页面，展示 `path_field` 和将要写回的路径；点击 `确认更新` 后调用 NocoDB v3 Data API。
- 上传会将所有选择项复制到目标目录：`current_path` 非空时必须是已有目录；为空时创建 `base_dir\\folder_name`，且该目录不能已存在。
- 上传在复制前检查全部来源和同名冲突；目标中已有同名文件或目录时整次失败，不覆盖、不改名。
- 上传成功后把目标目录绝对路径写回 `path_field`；如果 NocoDB 回写失败，文件保留在目标目录，GUI 会明确提示“文件已上传但路径未成功写回”。
- 更新成功后，同一个窗口里再次点击 `打开` 会使用刚更新的新路径。
- 更新或上传成功后，GUI 返回初始 `打开` / `更新` / `上传` 界面，不自动关闭。
- 只有 webhook 的 `sync_profile` 匹配到配置中的 `sync_profiles[].name` 时，GUI 才显示 `同步远端`；未传、空白或未知 profile 不会显示该按钮。
- 点击 `同步远端` 后，程序先读取本地 row 中的 lookup 字段，再用该值查询远端 NocoDB 记录。
- 同步过程中状态显示 `正在同步远端...`；同步成功后显示 `已同步远端字段。`，并且窗口保持打开。
- `同步远端` 只把 profile 中列出的 `sync_fields` 从远端记录更新回本地 row。
- `同步远端` 遇到 `远端未找到匹配记录`、`远端找到多条匹配记录`、本地查询字段为空或远端记录缺少任一同步字段时，不会更新本地 row。
- 打开、更新或上传失败时，错误会显示在 GUI 状态文本中，窗口不会自动关闭，用户可以重试或取消。
- 正在打开、更新或上传时，按钮会暂时禁用；此时关闭窗口会被阻止，避免中断正在进行的操作。

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
| 多次请求后 GUI 没有马上全部出现 | 这是 `max_gui_windows` 的限制行为；默认只同时显示 1 个窗口，不同 row 的其余请求等待。 |
| 同一行重复触发 `/webhook` 没有新窗口 | 该 row 已有 GUI 窗口或待打开请求；如果窗口已打开，程序会尝试把它拉到前台。 |
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
