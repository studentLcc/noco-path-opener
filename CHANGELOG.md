# Changelog

## v0.2.0 - 2026-05-19

- Add `POST /webhook` endpoint for queued NocoDB GUI actions.
- Add Windows GUI for opening the current row path or selecting a file/directory path.
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
