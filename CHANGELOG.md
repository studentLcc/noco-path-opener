# Changelog

## 0.5.0 - 2026-08-14

- Remove the legacy `sync_profiles` configuration, runtime synchronization path,
  and profile generator.
- Use `current_path` as the remote download directory; when empty, create
  `base_dir\folder_name` and write it back to `path_field`.
- Retain downloaded files and any newly created directory after a later sync
  failure.
- Add `remote_sync_headers.json` for remote POST, detail GET, and file download
  headers. `{token}` is replaced with the GUI synchronization key.
- Replace the modal token dialog with a persistent masked key input in the main
  action window.

## 0.4.0 - 2026-08-14

- Add webhook-defined `remote_sync` rules while keeping local reads and writes on the NocoDB v3 Data API.
- Add password-style `snc-token` input for each dynamic remote synchronization.
- Remember the token from the last successful synchronization in a Windows DPAPI-encrypted temporary file.
- Read the first POST body from `remote_sync_params.json` and inject `params.condition.processCode`.
- Dynamically locate the first `changedFormData`, map its configured input value, and collect every `file_upload*` item.
- Download remote files with URL placeholders into a directory read from the current NocoDB record.
- Prompt before using an existing remote download directory, with overwrite or
  skip attachment choices.
- Reject redirects, bound remote response sizes, and roll back files created by failed synchronization attempts.
- Document the complete NocoDB webhook contract in `docs/WEBHOOK.md`.
- Add a console debug build that bypasses tray initialization and logs remote synchronization requests without logging `snc-token`.
- Add configurable 1-120 second timeouts for the remote POST and detail GET requests.
- Keep dynamic and legacy synchronization windows separate for the same NocoDB row.

## 0.3.1 - 2026-08-14

- Support native multi-select file picking for uploads.
- Allow adding files and folders separately on the upload confirmation page.
- Deduplicate upload paths and remember the most recently selected directory.
- Fix the Windows folder-picker callback to use the callback data correctly.

## 0.3.0 - 2026-05-20

- Add optional remote field sync profiles selected by webhook `sync_profile`.
- Add conditional GUI `同步远端` action for matched sync profiles.
- Add NocoDB v3 read, query, and raw multi-field update helpers for remote field sync profiles.

## v0.2.0 - 2026-05-19

- Add `POST /webhook` endpoint for queued NocoDB GUI actions.
- Add Windows GUI for opening the current row path or selecting a file/directory path.
- Include `record_id` in Windows GUI window titles.
- Focus the existing GUI window for duplicate `/webhook` requests on the same row instead of creating a second GUI request.
- Add NocoDB v3 record update client for writing selected absolute paths back to a text field.
- Add local `nocodb_url` and `nocodb_token` config fields.
- Keep existing `POST /open` behavior compatible.

## v0.1.0 - 2026-05-19

- Initial Windows local path opener service.
- Add JSON config loading and default config generation.
- Add `POST /open` endpoint for opening existing files and directories.
- Add allowed roots authorization for limiting accessible paths.
- Add Windows opener boundary and non-Windows test/build stub.
- Add README usage documentation.
