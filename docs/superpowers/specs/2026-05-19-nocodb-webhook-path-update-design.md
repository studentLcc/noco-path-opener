# NocoDB Webhook Path Update Design

## Context

`noco-path-opener` currently exposes `POST /open`, accepts a local path, checks `allowed_roots`, and opens an existing file or directory on Windows. The new requirement is to configure one NocoDB webhook that always opens a local GUI. The user can either open the current local path from the NocoDB row or choose a file/directory path and update that value back to the row.

The webhook request must return quickly. GUI work, path selection, and NocoDB updates run after the HTTP response has already been accepted.

## Goals

- Add one NocoDB webhook endpoint that always opens a GUI action window.
- Keep the existing `/open` endpoint and behavior unchanged.
- Let the webhook payload provide `base_id`, `table_id`, `record_id`, `path_field`, and `current_path`.
- Store NocoDB URL and API token locally in `config.json`, not in the webhook payload.
- Let the user update a NocoDB text field with an absolute local file or directory path.
- Support file/directory drag and drop into the GUI.
- Show the GUI near the current mouse pointer when possible.
- Show success or error feedback to the user.

## Non-Goals

- Uploading file contents to NocoDB attachment fields.
- Adding token authentication to the local webhook server.
- Converting the app into a Windows service or tray app.
- Returning final GUI/update success to NocoDB webhook callers.

## HTTP API

Add a new endpoint:

```text
POST /webhook
```

`POST /open` remains compatible with the existing API and continues to directly open a supplied `path`.

### Webhook Payload

The NocoDB webhook should send JSON shaped like this:

```json
{
  "base_id": "p_xxxxx",
  "table_id": "m_xxxxx",
  "record_id": 123,
  "path_field": "本地文件路径",
  "current_path": "C:\\Users\\YourName\\Desktop\\old.docx"
}
```

Fields:

- `base_id`: NocoDB base id used in the v3 Data API URL.
- `table_id`: NocoDB table id, normally from the webhook event.
- `record_id`: Current row `Id`. The payload may provide this as a JSON number or string; the implementation should preserve a numeric id when supplied and support string ids if NocoDB uses them.
- `path_field`: Field name to update with the selected absolute path.
- `current_path`: Existing path value from the row. This may be empty.

Example NocoDB custom payload:

```json
{
  "base_id": "p_xxxxx",
  "table_id": {{ json event.data.table_id }},
  "record_id": {{ json event.data.rows.[0].Id }},
  "path_field": "本地文件路径",
  "current_path": {{ json event.data.rows.[0].本地文件路径 }}
}
```

### Webhook Response

If the JSON is valid and the required fields are present, the server returns immediately:

```json
{
  "success": true,
  "queued": true
}
```

Status code: `202 Accepted`.

Validation failures return JSON errors:

- `400` for invalid JSON.
- `400` for missing or empty `base_id`, `table_id`, `record_id`, or `path_field`.
- `404` for unknown routes.
- `405` for wrong methods on known routes.

The server does not check whether `current_path` exists before returning `202`, because that would couple webhook latency to local filesystem work.

## GUI Behavior

When `/webhook` accepts a request, the server starts a background GUI action flow.

Window title:

```text
文件操作
```

Main buttons:

```text
打开
上传或更新
取消
```

The window should appear near the current mouse pointer, offset slightly so it does not cover the pointer. If the mouse position cannot be read, the window falls back to a centered position.

### Open

`打开` uses `current_path` from the webhook payload.

Behavior:

- If `current_path` is empty, show an error or disabled state.
- Check `allowed_roots`.
- Check that the path exists.
- Reuse the existing Windows open behavior for files and directories.
- On success, show a short success message and close the UI.
- On failure, show an error message and keep the UI open.

### Upload Or Update

`上传或更新` updates the NocoDB path field with a local absolute path. It never uploads file contents.

Behavior:

- The user can drag a file or directory into the GUI, and the GUI displays the complete path.
- The user can also click `上传或更新` to open a picker.
- The picker must support selecting a file or directory path. If the chosen GUI library cannot provide one dialog that picks both, use a clear two-step or two-option picker inside the upload/update flow.
- Any selected or dropped path is converted to an absolute path.
- The selected path is checked against `allowed_roots`.
- Before sending the update, the GUI displays a confirmation view containing the complete path and the target field name.
- The user must confirm before the NocoDB PATCH request is sent.
- On success, show a short success message and close the UI.
- On failure, show an error message and keep the UI open so the user can retry, reselect, or cancel.

Confirmation view buttons:

```text
确认更新
重新选择
取消
```

`重新选择` opens the picker again. `取消` cancels the update flow without changing NocoDB.

### Cancel And Close

`取消` closes the UI without any filesystem or NocoDB action.

Closing the window is equivalent to canceling.

## Configuration

Extend `config.json`:

```json
{
  "host": "0.0.0.0",
  "port": 6666,
  "allowed_roots": [],
  "nocodb_url": "http://localhost:8080",
  "nocodb_token": ""
}
```

Fields:

- `nocodb_url`: Base URL for the NocoDB instance.
- `nocodb_token`: API token used for NocoDB API requests.

Existing config files that do not include these fields should remain valid. `/open` must still work without NocoDB settings. `/webhook` can accept requests without checking NocoDB settings up front, but update confirmation must fail with a clear GUI error if `nocodb_url` or `nocodb_token` is empty.

## NocoDB Update API

When the user confirms an update, call the NocoDB v3 Data API:

```text
PATCH {nocodb_url}/api/v3/data/{base_id}/{table_id}/records
xc-token: {nocodb_token}
Content-Type: application/json
```

Request body:

```json
{
  "id": 123,
  "fields": {
    "本地文件路径": "C:\\Users\\YourName\\Desktop\\a.docx"
  }
}
```

`path_field` from the webhook is used as the field name. The selected absolute path is used as the field value.

Non-2xx responses are treated as update failures. The app logs the status code and a short response summary, then shows a user-facing error in the GUI.

## Components

The implementation should keep GUI, HTTP routing, filesystem opening, path authorization, and NocoDB API calls separated behind small interfaces.

Recommended boundaries:

- `internal/openapi`: HTTP routes, request validation, response JSON, background dispatch.
- `internal/pathauth`: existing `allowed_roots` validation.
- `internal/winopen`: existing Windows open operation.
- `internal/nocodb`: NocoDB v3 record update client.
- `internal/gui`: Windows GUI flow for action selection, drag/drop, picker, confirmation, and mouse positioning.
- `internal/actions`: orchestration between GUI decisions, path checks, opener, and NocoDB client.

The HTTP layer should depend on an action dispatcher interface so tests can verify that `/webhook` queues work without showing real UI.

## Error Handling

- Invalid `/webhook` request: return JSON error before starting background work.
- Missing NocoDB config during update: show GUI error and log it.
- Empty `current_path`: show GUI error or disabled `打开` behavior.
- Disallowed path: show GUI error and do not open or update.
- Missing path: show GUI error and do not call the opener.
- File picker canceled: return to the main GUI state without updating.
- NocoDB timeout or non-2xx response: show GUI error and keep the UI open.
- Background errors after `202` are never sent back to the webhook caller.

## Testing

Automated tests should avoid opening desktop UI.

Test coverage:

- Existing `/open` tests continue to pass.
- `/webhook` rejects invalid JSON and missing required fields.
- Valid `/webhook` returns `202 Accepted` and calls a fake dispatcher.
- NocoDB client sends the expected method, URL, `xc-token` header, content type, and JSON body.
- Action orchestration checks `allowed_roots` before opening or updating.
- Action orchestration converts selected/dropped paths to absolute paths before update.
- Failed open or update operations surface errors through the GUI interface.

Manual Windows verification:

- NocoDB webhook opens the `文件操作` window near the mouse pointer.
- `打开` opens the current path and closes after success feedback.
- Dragging a file displays the full path, confirms, updates NocoDB, and closes after success feedback.
- Dragging a directory follows the same update flow.
- Clicking `上传或更新` opens a picker, confirms the selected path, updates NocoDB, and closes after success feedback.
