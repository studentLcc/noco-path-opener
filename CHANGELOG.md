# Changelog

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
