# NocoDB Remote Field Sync Design

## Context

`noco-path-opener` already accepts `POST /webhook`, opens a Windows GUI for the current NocoDB row, and lets the user either open the row's local path or update a local path field back to NocoDB. The new requirement adds a third optional GUI action: sync selected fields from another NocoDB server into the current local row.

The local and remote tables do not necessarily have identical schemas. A sync profile defines how to find the remote row and which same-named fields are safe to copy back.

## Goals

- Keep the existing `/open` and `/webhook` path workflows compatible.
- Let a NocoDB webhook choose a sync profile by name using `sync_profile`.
- Show the `同步远端` button only when the webhook names an existing sync profile.
- Use a profile-level safety check to ensure the webhook's local `base_id/table_id` matches the intended local table.
- Read the local row from the local NocoDB server to get the lookup value.
- Query one shared remote NocoDB server for the matching remote row.
- Update only configured same-named fields back to the current local row.
- Preserve normal JSON values for common field types such as text, date, single select, number, and boolean.
- Avoid partial updates when the remote row is ambiguous or missing required sync fields.

## Non-Goals

- Automatically syncing every same-named field.
- Supporting multiple remote NocoDB servers per sync profile.
- Syncing attachments, linked records, users, or other complex NocoDB field types.
- Running sync immediately when the webhook is received.
- Adding local HTTP authentication.
- Returning final sync success or failure to the webhook caller.

## Configuration

Extend `config.json` with one shared remote NocoDB connection and zero or more sync profiles:

```json
{
  "host": "0.0.0.0",
  "port": 6666,
  "allowed_roots": [],
  "max_gui_windows": 1,

  "nocodb_url": "http://local-nocodb:8080",
  "nocodb_token": "LOCAL_TOKEN",

  "remote_nocodb_url": "https://remote-nocodb.example.com",
  "remote_nocodb_token": "REMOTE_TOKEN",

  "sync_profiles": [
    {
      "name": "change-log-main",
      "local_base_id": "p_local_main",
      "local_table_id": "m_local_change_log",
      "local_lookup_field": "变更单号",

      "remote_base_id": "p_remote_main",
      "remote_table_id": "m_remote_change_log",
      "remote_lookup_field": "变更单号",

      "sync_fields": ["状态", "负责人", "计划完成时间"]
    },
    {
      "name": "project-a-change-log",
      "local_base_id": "p_local_project_a",
      "local_table_id": "m_local_project_a_change_log",
      "local_lookup_field": "工单编号",

      "remote_base_id": "p_remote_project_a",
      "remote_table_id": "m_remote_project_a_change_log",
      "remote_lookup_field": "变更单号",

      "sync_fields": ["状态", "优先级", "上线时间"]
    }
  ]
}
```

Validation rules:

- Existing config files without remote sync fields remain valid.
- `remote_nocodb_url` and `remote_nocodb_token` are required only when `sync_profiles` is non-empty.
- Each profile `name` must be non-empty and unique.
- `local_base_id`, `local_table_id`, `local_lookup_field`, `remote_base_id`, `remote_table_id`, and `remote_lookup_field` must be non-empty.
- `sync_fields` must contain at least one non-empty field name.
- Field names in one profile's `sync_fields` must be unique after trimming whitespace.

## Webhook Payload

`POST /webhook` keeps the existing fields and adds optional `sync_profile`:

```json
{
  "base_id": "p_local_main",
  "table_id": "m_local_change_log",
  "record_id": 123,
  "path_field": "本地文件路径",
  "current_path": "D:\\a.docx",
  "sync_profile": "change-log-main"
}
```

`path_field` and `current_path` continue to serve the existing open/update-path workflow.

`sync_profile` behavior:

- Missing field: no remote sync capability is attached to the GUI.
- Empty or whitespace-only value: no remote sync capability is attached to the GUI.
- Unknown profile name: no remote sync capability is attached to the GUI.
- Known profile name: the GUI receives that profile as an optional sync capability.

The HTTP response remains `202 Accepted` after the request passes existing validation. A missing, empty, or unknown `sync_profile` does not make `/webhook` fail, because the desired behavior is to hide the sync button.

## GUI Behavior

The GUI keeps the existing buttons:

```text
打开
上传或更新
取消
```

When the webhook names an existing sync profile, the action row adds:

```text
同步远端
```

When there is no resolved sync profile, the `同步远端` button is not created. This includes the cases where `sync_profile` is not sent, is blank, or names a profile that does not exist.

Clicking `同步远端` starts an asynchronous sync operation:

- Disable action buttons while sync is running.
- Show `正在同步远端...` in the status label.
- On success, show `已同步远端字段。`.
- On failure, show a concise error and keep the window open.
- Do not close the window automatically after a successful sync, so the user can still open the path or update the path.

## Sync Data Flow

The sync operation runs only after the user clicks `同步远端`.

1. Check that the resolved profile's `local_base_id/local_table_id` match the webhook's `base_id/table_id`.
2. Read the current local row from `nocodb_url/nocodb_token` using the webhook's `base_id/table_id/record_id`.
3. Extract the local lookup value from `local_lookup_field`.
4. If the lookup value is empty, stop with `本地查询字段为空`.
5. Query the remote NocoDB server using `remote_base_id/remote_table_id` and a `where` filter where `remote_lookup_field` equals the local lookup value.
6. If the remote query returns zero rows, stop with `远端未找到匹配记录`.
7. If the remote query returns more than one row, stop with `远端找到多条匹配记录`.
8. From the single remote row, extract every field listed in `sync_fields`.
9. If any configured sync field is missing from the remote row, stop and list the missing fields. Do not update any local field.
10. PATCH the current local row with a `fields` object containing only the configured sync fields and the remote row's raw JSON values.

The sync is all-or-nothing at the application level. The program prepares the complete field map before sending the local update.

## NocoDB API

The existing local update path uses NocoDB v3 Data API. Remote field sync extends the client with three operations:

- Read one local record by `base_id/table_id/record_id`.
- Query remote records by `base_id/table_id/where`.
- Update one local record with multiple raw JSON field values.

Record update keeps the existing v3 endpoint shape:

```text
PATCH {nocodb_url}/api/v3/data/{base_id}/{table_id}/records
xc-token: {token}
Content-Type: application/json
```

The multi-field update body preserves raw JSON values:

```json
{
  "id": 123,
  "fields": {
    "状态": "已完成",
    "负责人": "张三",
    "计划完成时间": "2026-05-20"
  }
}
```

The remote query uses NocoDB v3 Data API `where` filtering. Field names and values must be quoted or escaped according to NocoDB's documented filter syntax so Chinese field names, spaces, and punctuation in lookup values are handled correctly.

Reference: <https://nocodb.com/docs/product-docs/developer-resources/rest-apis>

## Component Boundaries

Keep the current separation:

- `internal/config`: config structs, defaults, and validation for remote connection and sync profiles.
- `internal/openapi`: parse `sync_profile` from the webhook and pass the profile name into `actions.Request`.
- `internal/actions`: resolve profile names, decide whether remote sync is available, orchestrate local read, remote query, field extraction, and local update.
- `internal/nocodb`: implement v3 Data API read/query/update helpers.
- `internal/gui`: render `同步远端` only when the request includes a resolved sync capability and call the controller method when clicked.
- `cmd/noco-path-opener`: construct local and remote NocoDB clients and inject sync configuration into the action flow.

The HTTP layer should not fail unknown `sync_profile` values. The action layer can hide that capability before the GUI is shown by resolving the profile name during request handling.

## Error Handling

User-facing sync errors:

- Profile table mismatch: `同步配置与当前表不匹配`.
- Local row read fails: show the NocoDB status/body summary.
- Local lookup field is missing or empty: `本地查询字段为空`.
- Remote query fails: show the NocoDB status/body summary.
- Remote query returns zero rows: `远端未找到匹配记录`.
- Remote query returns multiple rows: `远端找到多条匹配记录`.
- Remote row lacks configured sync fields: show the missing field names and do not update.
- Local update fails: show the NocoDB status/body summary.

Unknown or blank `sync_profile` is not a sync error because the button is hidden.

## Testing

Automated tests should avoid desktop UI.

Config tests:

- Existing minimal config remains valid.
- Remote URL/token are required when profiles exist.
- Profile names must be unique.
- Required profile fields are validated.
- `sync_fields` must be non-empty and unique.

Webhook tests:

- Missing `sync_profile` preserves existing behavior.
- Blank `sync_profile` is accepted and treated as no sync capability.
- Non-blank `sync_profile` is passed into the action request.
- Unknown profile names do not produce HTTP errors.

Action tests:

- Unknown or blank profile results in no sync button capability.
- Known profile resolves and enables sync capability.
- Profile table mismatch returns the expected error when sync is clicked.
- Empty local lookup value returns the expected error.
- Remote zero-row and multi-row results return expected errors.
- Missing remote sync fields prevent any local update.
- Successful sync PATCHes only configured fields with raw JSON values.

NocoDB client tests:

- Local record read sends the expected method, URL, and `xc-token`.
- Remote query sends the expected method, URL, `xc-token`, and encoded `where` parameter.
- Multi-field update sends the expected PATCH body and preserves raw JSON field values.
- Non-2xx responses include status and a short response summary.

Manual Windows verification:

- Old webhook payloads show only `打开`, `上传或更新`, and `取消`.
- Blank or unknown `sync_profile` hides `同步远端`.
- Known `sync_profile` shows `同步远端`.
- Clicking `同步远端` shows running status and disables buttons.
- Successful sync updates the local NocoDB row and leaves the window open.
- Remote no-match, multi-match, table mismatch, and missing-field cases show clear GUI errors.
