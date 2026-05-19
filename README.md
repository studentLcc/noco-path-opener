# Noco Path Opener

Noco Path Opener 是一个运行在 Windows 上的小型 Go HTTP 服务，用来接收 NocoDB webhook 请求，并打开本机已有的文件或目录。

当前版本：`0.2.0`

典型场景：NocoDB 在 Docker 容器里运行，表格按钮触发 webhook，把 Windows 绝对路径发给本服务；本服务在 Windows 主机上用默认应用打开文件，或用文件资源管理器打开目录。

## 功能

- 接收 `POST /open` JSON 请求
- 打开已有文件，使用 Windows 默认关联应用
- 打开已有目录，使用 Windows 文件资源管理器
- 自动生成本地 `config.json`
- 支持 `allowed_roots` 限制可打开的根目录
- 返回 JSON 格式的成功和错误响应
- 自动化测试不会真的启动桌面应用
- 接收 `POST /webhook`，打开本机 GUI 操作窗口
- 可从 GUI 打开 NocoDB 行里的当前路径
- 可拖放或选择文件/目录，并把绝对路径更新回 NocoDB 文本字段
- NocoDB URL 和 API token 保存在本地 `config.json`

## 快速开始

### 1. 构建 Windows 可执行文件

在 WSL 或 Linux 环境中运行：

```bash
GOOS=windows GOARCH=amd64 go build -o noco-path-opener.exe ./cmd/noco-path-opener
```

如果 Go 不在 `PATH` 中，可以使用 Go 可执行文件的绝对路径，例如：

```bash
GOOS=windows GOARCH=amd64 /home/ccamj/go/bin/go build -o noco-path-opener.exe ./cmd/noco-path-opener
```

### 2. 放到 Windows 目录

把 `noco-path-opener.exe` 复制到 Windows 上你希望运行的位置，例如桌面或一个固定工具目录。

### 3. 双击运行

双击 `noco-path-opener.exe`。首次启动时，会在 exe 同目录生成 `config.json`：

```json
{
  "host": "0.0.0.0",
  "port": 6666,
  "allowed_roots": [],
  "nocodb_url": "http://localhost:8080",
  "nocodb_token": ""
}
```

控制台会输出类似：

```text
noco-path-opener listening on http://0.0.0.0:6666/open and http://0.0.0.0:6666/webhook
```

## NocoDB Webhook 配置

如果 NocoDB 运行在 Docker 容器中，请从容器访问 Windows 主机地址：

推荐把 NocoDB webhook 配置到 `/webhook`，它会打开本机 GUI 并把操作排队；HTTP 响应只表示请求已排队。

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

兼容的 `/open` 接口适合直接打开 payload 里提供的路径，不会打开 GUI，也不会把路径更新回 NocoDB。

```text
POST http://host.docker.internal:6666/open
Content-Type: application/json
```

请求体示例：

```json
{
  "path": "C:\\Users\\YourName\\Desktop\\test.txt"
}
```

目录也可以打开：

```json
{
  "path": "C:\\Users\\YourName\\Documents"
}
```

## API

### `POST /open`

请求体：

```json
{
  "path": "D:\\docs\\a.docx"
}
```

成功响应：

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
| `400` | JSON 无效，或 `path` 缺失/为空 |
| `403` | 路径不在 `allowed_roots` 允许范围内 |
| `404` | 路径不存在，或路由不存在 |
| `405` | `/open` 使用了非 POST 方法 |
| `500` | 路径存在，但 Windows 打开失败 |

### `POST /webhook`

NocoDB webhook 可以调用这个接口打开本机 GUI。HTTP 响应只表示请求已排队，不代表用户后续打开或更新一定成功。

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

状态码是 `202 Accepted`。请求 JSON 无效，或缺少 `base_id`、`table_id`、`record_id`、`path_field` 时返回 `400`。

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

## 配置

`config.json` 必须放在 `noco-path-opener.exe` 同目录。

```json
{
  "host": "0.0.0.0",
  "port": 6666,
  "allowed_roots": [],
  "nocodb_url": "http://localhost:8080",
  "nocodb_token": ""
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `host` | HTTP 监听地址，默认 `0.0.0.0`，方便 Docker 容器访问宿主机 |
| `port` | HTTP 监听端口，默认 `6666` |
| `allowed_roots` | 允许打开的根目录列表；为空数组时允许所有路径 |
| `nocodb_url` | NocoDB 实例地址，例如 `http://localhost:8080` |
| `nocodb_token` | NocoDB API token；只保存在本机配置中，不放进 webhook payload |

限制可打开目录示例：

```json
{
  "host": "0.0.0.0",
  "port": 6666,
  "allowed_roots": [
    "C:\\Users\\YourName\\Documents",
    "D:\\Work"
  ],
  "nocodb_url": "http://localhost:8080",
  "nocodb_token": ""
}
```

配置无效时，程序会在控制台打印错误并退出。

## 安全提醒

默认配置监听 `0.0.0.0:6666`，并且没有 token 认证。这是为了让 Docker 里的 NocoDB 能访问 Windows 主机，但也可能让同一网络里的其他设备访问该接口，具体取决于 Windows 防火墙规则。

建议：

- 只在可信网络中运行
- 用 Windows 防火墙限制访问来源
- 使用 `allowed_roots` 限制可打开的目录
- `nocodb_token` 以明文保存在本机 `config.json` 中，请保护该文件的访问权限
- 不要把端口暴露到公网

## 手动验证

在 Windows 上启动程序后，可以从 NocoDB 容器或其他终端发送请求：

```bash
curl -i -X POST http://host.docker.internal:6666/open \
  -H 'Content-Type: application/json' \
  -d '{"path":"C:\\Users\\YourName\\Desktop\\test.txt"}'
```

也可以验证错误响应：

```bash
curl -i -X POST http://host.docker.internal:6666/open \
  -H 'Content-Type: application/json' \
  -d '{"path":"   "}'
```

```bash
curl -i -X POST http://host.docker.internal:6666/open \
  -H 'Content-Type: application/json' \
  -d '{"path":"C:\\path\\that\\does\\not\\exist.txt"}'
```

## Webhook GUI 行为

`/webhook` 接收请求后会打开标题为 `文件操作` 的窗口。窗口包含 `打开`、`上传或更新`、`取消`。

- `打开` 会检查 `current_path` 是否为空、是否在 `allowed_roots` 内、是否存在，然后用现有 Windows 打开逻辑打开文件或目录。
- `上传或更新` 会让用户选择文件或目录，也可以直接拖放文件/目录到窗口中。
- 选择或拖放的路径会转换成绝对路径，检查 `allowed_roots` 和本地存在性，然后进入确认页面。
- 点击 `确认更新` 后，程序会调用 NocoDB v3 Data API，把 `path_field` 指定的文本字段更新为本地绝对路径。
- 任何打开或更新失败都会显示在 GUI 中，窗口不会立即关闭，用户可以重试或取消。

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
GOOS=windows GOARCH=amd64 go build -o noco-path-opener.exe ./cmd/noco-path-opener
```

项目结构：

```text
cmd/noco-path-opener/      程序入口
internal/actions/          webhook GUI 动作编排
internal/config/           配置读取、默认配置生成、配置校验
internal/gui/              Windows GUI 和非 Windows stub
internal/nocodb/           NocoDB v3 Data API 客户端
internal/openapi/          HTTP 路由、请求校验、JSON 响应
internal/pathauth/         allowed_roots 路径授权判断
internal/winopen/          Windows 打开文件/目录的边界实现
```

## 已知限制

- 不是 Windows 服务，需要手动启动
- 没有托盘图标
- 没有 token 认证
- 不限制文件扩展名
- Linux/WSL 上只能验证 HTTP 和 JSON 行为，不能真的打开 Windows 桌面应用
