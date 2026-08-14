# Dynamic Remote Sync Simplification Design

## Goal

Replace the legacy `sync_profiles` synchronization path with a webhook-defined
`remote_sync` flow. Use `current_path` as the only download destination field,
move remote request headers into an external template file, and keep the
authentication key visible in the main action window.

## Scope

- Remove legacy remote NocoDB configuration and all `sync_profiles` runtime,
  configuration, CLI generator, tests, and documentation.
- Accept only dynamic `remote_sync` settings in `/webhook`.
- Read request headers from `remote_sync_headers.json` beside the executable.
- Show a masked synchronization key input in the main window whenever dynamic
  remote synchronization is available.
- Use the key input value when the user clicks `同步远端`; do not show a
  separate token dialog.
- Reuse the entered key for future synchronization attempts in the same window.
- Persist the entered key with the existing Windows DPAPI store so a later
  action window starts with the previous value.
- Use `current_path` as the remote download directory when it is non-empty.
- When `current_path` is empty, create `base_dir\folder_name`, download there,
  and write that absolute directory to `path_field`.
- Retain any files and a newly created directory when a remote synchronization
  later fails. Existing files are never overwritten.

## Webhook Contract

The `remote_sync` object removes `download_directory_field`. Its
`field_mapping` maps supported remote source keys to local NocoDB column names.

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
    "field_mapping": {
      "name": "远程名称",
      "id": "远程ID",
      "designName": "设计名称",
      "input_value": "表单输入值",
      "file_uploads": "远程附件JSON"
    }
  }
}
```

The only supported `field_mapping` source keys are `name`, `id`,
`designName`, `input_value`, and `file_uploads`. Each value is the exact local
NocoDB column name.

## External Header Template

`remote_sync_headers.json` is stored beside the executable. It is a JSON
object of HTTP header names and values:

```json
{
  "snc-token": "{token}",
  "X-App-Code": "your-app-code"
}
```

Every occurrence of `{token}` is replaced with the trimmed value in the
visible synchronization key input. The resulting headers are attached to the
remote POST, detail GET, and file download requests. A missing, invalid, or
empty-value template produces a visible synchronization error before any
remote request is made.

The existing `remote_sync_params.json` remains the external POST body template.

## Download and Write-Back Flow

1. The user clicks `同步远端`; the app reads the key input and header template.
2. The app fetches and maps remote values.
3. It chooses the download directory:
   - A non-empty `current_path` must be an allowed existing directory.
   - Otherwise `base_dir` must be an allowed existing directory and
     `folder_name` must be valid. The app creates `base_dir\folder_name`.
4. The app preflights all remote file names and refuses any collision with an
   existing destination file.
5. It downloads files without overwriting.
6. It patches mapped fields to the local record. If a directory was created
   for this synchronization, the same patch also sets `path_field` to that
   directory and updates the in-window `current_path`.
7. If a later download or NocoDB patch fails, files and a directory already
   created in this attempt remain in place. The error is displayed to the
   user.

## UI Behavior

The main action window shows a masked `同步 Key` input whenever the webhook has
a valid `remote_sync` object. It is initialized from the DPAPI store when a
saved key exists. `同步远端` reads the current field value directly. It does not
open a modal authentication dialog.

## Test Plan

- Configuration default and validation no longer reference legacy profiles.
- Webhook rejects legacy `sync_profile` input as unsupported or ignores it
  without activating any legacy behavior.
- Header template loads correctly, substitutes `{token}`, and reaches POST,
  GET, and download requests.
- Missing or malformed header templates fail before a remote request.
- Dynamic synchronization uses `current_path` and does not read a separate
  NocoDB directory field.
- Empty `current_path` creates `base_dir\folder_name`, writes it to
  `path_field`, and retains files/directories on later failure.
- Source tests confirm the main GUI contains the persistent masked key input
  and has no modal token prompt.
- The full Go test suite and Windows GUI/CLI cross-builds pass.
