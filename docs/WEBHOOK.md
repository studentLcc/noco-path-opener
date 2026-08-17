# NocoDB Webhook Specification

`POST /webhook` accepts one NocoDB record and an optional dynamic
`remote_sync` rule. Valid requests return `202 Accepted`; remote work happens
later in the Windows GUI.

## Template

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

## Remote Sync Fields

| Field | Required | Description |
| --- | --- | --- |
| `post_url` | Yes | First remote POST endpoint |
| `get_url` | Yes | Detail endpoint containing `{id}` |
| `download_url` | Yes | File endpoint containing a file ID and name placeholder |
| `processCode` | Yes | Value written to `params.condition.processCode` |
| `input_field` | Yes | `changedFormData` field whose `value` is mapped |
| `request_timeout_seconds` | No | POST and detail GET timeout, 1-120 seconds |
| `field_mapping` | Yes | Remote source key to local NocoDB column name |

`download_url` accepts `{file_id}` or `{id}`, and `{file_name}` or `{name}`.
All placeholder values are URL-encoded.

Supported `field_mapping` keys are `name`, `id`, `designName`, `creator`,
`input_value`, and `file_uploads`.

`input_field` is the remote field name to read. If it is missing from
`changedFormData`, `input_value` is skipped without failing the synchronization;
other mapped values and attachments continue normally.

## External Files

The repository and release archive provide
`remote_sync_params.example.json` and `remote_sync_headers.example.json`.
The executable directory uses the corresponding filenames without `.example`:

`remote_sync_params.json`:

```json
{
  "params": {
    "condition": {
      "processCode": "xxxx"
    }
  }
}
```

`remote_sync_headers.json`:

```json
{
  "snc-token": "{token}",
  "X-App-Code": "your-app-code"
}
```

`{token}` is replaced by the masked `同步 Key` input shown in the main action
window. The resolved headers are applied to the POST, detail GET, and file
downloads.

## Download Rule

When attachments are present, `current_path` is used as the local download
directory. It must be an existing allowed directory.

When `current_path` is empty, `base_dir` must be an existing allowed directory
and `folder_name` must be a valid Windows directory name. The app creates
`base_dir\folder_name`, downloads files there, and writes that directory to
`path_field` with the mapped fields.

If the download directory already exists, the GUI asks whether to overwrite
same-named attachments or skip attachment downloads. Overwrite only replaces
same-named regular files; unrelated files in the directory are preserved.
Skip leaves all attachments untouched and still updates mapped remote fields.
If a download or the NocoDB PATCH fails after files or a directory have been
created, those local files remain.
