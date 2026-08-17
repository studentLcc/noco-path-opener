# Noco Path Opener

Noco Path Opener runs on a Windows desktop session. It accepts NocoDB webhooks,
opens local paths, uploads selected files, and synchronizes remote process data
back to the triggering NocoDB record.

Current version: `0.5.0`

## Features

- `POST /open` opens an existing local file or directory.
- `POST /webhook` queues a Windows GUI for one NocoDB record.
- The GUI can open, update, or upload local paths.
- Dynamic `remote_sync` reads remote data and files, then updates the local
  NocoDB record through the v3 Data API.
- The synchronization key is always visible as a masked input in the main GUI;
  no separate authentication dialog is used.
- Remote HTTP headers come from `remote_sync_headers.json`.
- `current_path` is the download directory. If it is empty, the app creates
  `base_dir\folder_name` and writes that path back to `path_field`.
- When a remote download directory already exists, the GUI asks whether to
  overwrite same-named attachments or skip attachment downloads. Files and a
  newly created directory remain in place if a later download or NocoDB update
  fails.

## Build

Windows GUI build:

```bash
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o noco-path-opener.exe ./cmd/noco-path-opener
```

Console debug build:

```bash
GOOS=windows GOARCH=amd64 go build -tags consoledebug -trimpath -o noco-path-opener-cli.exe ./cmd/noco-path-opener
```

Release build:

```bash
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H windowsgui" -o dist/noco-path-opener.exe ./cmd/noco-path-opener
```

The console build logs webhook receipt, effective request headers, computed
`Content-Length`, JSON request bodies, and JSON response bodies for the remote
POST and detail GET. Token, authorization, cookie, API-key, secret, and
password Header values are redacted; downloaded file content is never logged.

## Local Files

The repository and release archive provide `config.example.json`,
`remote_sync_params.example.json`, and `remote_sync_headers.example.json`.
The executable directory uses the corresponding filenames without `.example`.

| File | Required when | Purpose |
| --- | --- | --- |
| `config.json` | Always | Local listener and local NocoDB connection; generated on first launch when absent |
| `remote_sync_params.json` | Using `remote_sync` | First remote POST body template |
| `remote_sync_headers.json` | Using `remote_sync` | Remote request header template |

Default `config.json`:

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

`allowed_roots` is optional. An empty array permits every local path, so use it
only on a trusted machine and network.

`remote_sync_headers.json` is a JSON object. `{token}` is replaced with the
masked `同步 Key` field in the GUI:

```json
{
  "snc-token": "{token}",
  "X-App-Code": "your-app-code"
}
```

The final headers are sent to the remote POST, detail GET, and every file
download. For JSON POSTs, `Content-Type: application/json` is added when the
template does not define it.

`remote_sync_params.json` example:

```json
{
  "params": {
    "condition": {
      "processCode": "xxxx"
    },
    "pagination": {
      "pagenum": 1,
      "pagesize": 20,
      "sort": "null"
    }
  }
}
```

## NocoDB Webhook

For NocoDB in Docker Desktop, set the webhook URL to:

```text
POST http://host.docker.internal:6666/webhook
Content-Type: application/json
```

Use this custom payload template and replace example field names and remote
URLs with your own:

```text
{
  "base_id": "p_xxxxx",
  "table_id": {{ json event.data.table_id }},
  "record_id": {{ json event.data.rows.[0].Id }},
  "path_field": "本地文件路径",
  "current_path": {{ json event.data.rows.[0].本地文件路径 }},
  "base_dir": {{ json event.data.rows.[0].基础目录 }},
  "folder_name": {{ json event.data.rows.[0].项目编号 }},
  "remote_sync": {
    "post_url": "https://remote.example.com/api/process/list",
    "get_url": "https://remote.example.com/api/process/{id}",
    "download_url": "https://remote.example.com/api/file/{file_id}?name={file_name}",
    "processCode": {{ json event.data.rows.[0].流程编码 }},
    "input_field": "input",
    "request_timeout_seconds": 10,
    "field_mapping": {
      "name": "远程名称",
      "id": "远程ID",
      "designName": "设计名称",
      "creator": "创建人",
      "input_value": "表单输入值",
      "file_uploads": "远程附件JSON"
    }
  }
}
```

`field_mapping` maps a fixed remote source key to an exact local NocoDB column
name:

| Source key | Remote value |
| --- | --- |
| `name` | `data.records[0].name` |
| `id` | `data.records[0].id` |
| `designName` | `data.records[0].designName` |
| `creator` | `data.records[0].creator` |
| `input_value` | First matching `changedFormData.<input_field>.value` |
| `file_uploads` | JSON array of extracted `{id,name}` attachments |

Each mapping target must be unique. Omit keys that the local table does not
need. `input_field` identifies the remote field to read; if that field is
missing from `changedFormData`, the app skips `input_value` without failing and
continues synchronizing other mapped values and attachments.

## Remote Synchronization Flow

1. NocoDB sends the webhook and receives `202 Accepted`.
2. The app shows the main action window. Enter or update `同步 Key`.
3. Click `同步远端`; the app reads `remote_sync_headers.json` and replaces
   `{token}` with the current input value.
4. The app posts `remote_sync_params.json` with
   `params.condition.processCode` replaced by the webhook value.
5. It fetches the detail response, maps values, and extracts every
   `file_upload*` attachment.
6. If attachments exist, it uses `current_path` as the download directory.
   If `current_path` is empty, it creates `base_dir\folder_name`.
7. When the download directory already exists, the GUI asks whether to
   overwrite same-named attachments or skip attachments and only PATCH mapped
   fields. It then PATCHes mapped fields to the same local NocoDB record. A
   directory selected for downloads while `current_path` is empty is written
   to `path_field` in that PATCH.

The entered key is encrypted with Windows DPAPI and prefilled in later action
windows. It is never written to the webhook, NocoDB, normal logs, or
`config.json`.

## API

### `POST /open`

```json
{
  "path": "D:\\docs\\a.docx"
}
```

### `POST /webhook`

Required top-level fields:

| Field | Description |
| --- | --- |
| `base_id` | Local NocoDB base/project ID |
| `table_id` | Local NocoDB table ID |
| `record_id` | Local NocoDB record ID |
| `path_field` | Local NocoDB field used to store the local path |

Optional path fields:

| Field | Description |
| --- | --- |
| `current_path` | Existing Windows directory used for open/upload/download |
| `base_dir` | Parent directory used when `current_path` is empty |
| `folder_name` | New subdirectory name when `current_path` is empty |

Successful webhook receipt returns:

```json
{
  "success": true,
  "queued": true
}
```

See [docs/WEBHOOK.md](docs/WEBHOOK.md) for the complete dynamic synchronization
contract.

## Development

```bash
go test ./...
go build ./cmd/noco-path-opener
```

The Windows GUI only runs in a Windows desktop session. Linux and WSL can run
the HTTP, configuration, and source-level tests.
