# Sync Profile Generator Design

## Goal

Add a separate interactive command-line tool that helps create `sync_profiles` entries for `config.json`. The tool reads existing NocoDB connection settings from the config file, asks the user for the local and remote table IDs, reads field metadata from NocoDB, lets the user choose lookup and sync fields, and then outputs a valid `config.SyncProfile`.

The tool is separate from the tray service so one-time configuration work does not complicate the long-running desktop program.

## Scope

In scope:

- New command under `cmd/noco-sync-profile-gen`.
- Default behavior prints one profile JSON object to stdout.
- Optional `-config <path> -write` mode appends the generated profile to `sync_profiles` in an existing config file.
- Credentials are read from `config.json` first:
  - local: `nocodb_url`, `nocodb_token`
  - remote: `remote_nocodb_url`, `remote_nocodb_token`
- Missing connection values are prompted interactively and kept in memory.
- The user manually enters local and remote `base_id/table_id`.
- The tool uses NocoDB Meta API field metadata to list fields.
- `sync_fields` are selected from the remote table field list and must exist by name in the local table.

Out of scope:

- Integrating the wizard into `noco-path-opener.exe`.
- Discovering bases or tables automatically.
- Writing or changing NocoDB tokens in `config.json`.
- Supporting offline/manual field entry in the initial version.
- Building a TUI with arrow-key selection.

## User Interface

Supported invocation:

```bash
noco-sync-profile-gen -config config.json
noco-sync-profile-gen -config config.json -write
```

If `-config` is omitted, the tool uses `config.json` in the current working directory. The config file must exist and be valid JSON; only missing connection values inside the config are prompted interactively.

The wizard prompts for:

1. Missing local and remote NocoDB URL/token values.
2. Profile name.
3. Local `base_id`.
4. Local `table_id`.
5. Remote `base_id`.
6. Remote `table_id`.
7. Local lookup field, selected by number from the local field list.
8. Remote lookup field, selected by number from the remote field list.
9. Sync fields, selected by comma-separated numbers from the remote field list.

Lookup field selection accepts exactly one number. Sync field selection accepts one or more comma-separated numbers such as `1,3,5`. The tool rejects empty input, out-of-range indexes, and duplicate selections.

## Architecture

### Command Package

`cmd/noco-sync-profile-gen` owns CLI parsing and process exit behavior. It should keep `main.go` thin:

- Parse `-config` and `-write`.
- Open stdin/stdout/stderr.
- Call an internal generator package.
- Print user-facing errors and exit non-zero on failure.

### Generator Package

Add a focused internal package, for example `internal/profilegen`, with three responsibilities:

- Interactive prompting and selection parsing.
- Profile generation and validation.
- Optional config-file append.

The package should avoid depending on terminal-specific features. Plain line-oriented input is enough and works in Windows consoles and automated tests.

### NocoDB Metadata Client

Extend `internal/nocodb` or add a small metadata client in `internal/profilegen` to read table fields from NocoDB Meta API. The client should return a normalized list of fields:

```go
type Field struct {
    ID    string
    Name  string
    Title string
}
```

Profile generation uses the field display name that NocoDB users see. The exact response decoder should tolerate common field-name keys such as `title`, `column_name`, or `name`, but generation must fail if no usable field name can be found.

The metadata client must include `xc-token` and should report non-2xx responses with the operation, status code, and a short response body summary.

## Data Flow

1. Load and parse the config file.
2. Resolve local and remote credentials from config values or prompts.
3. Prompt for profile name and table identifiers.
4. Fetch local fields from local NocoDB.
5. Fetch remote fields from remote NocoDB.
6. Prompt for local lookup field from local fields.
7. Prompt for remote lookup field from remote fields.
8. Prompt for sync fields from remote fields.
9. Verify every selected sync field exists by the same name in the local field list.
10. Build:

```json
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
```

11. In default mode, print the profile JSON object.
12. In `-write` mode, append the profile to `sync_profiles`, validate the whole config with existing config validation, and write formatted JSON back to the config file.

## Validation And Error Handling

The tool should fail before producing output when:

- The config file cannot be read or parsed.
- A required URL/token remains empty after prompting.
- A profile name is empty.
- Any base or table ID is empty.
- Meta API requests fail.
- A table returns no usable fields.
- Lookup selection is empty or invalid.
- Sync field selection is empty, invalid, or duplicated.
- A selected sync field does not exist in the local table by the same name.
- `-write` would create a duplicate profile name.
- The final config fails `config.Config.Validate()`.
- The config file cannot be written.

The tool should not write NocoDB tokens back to config when they were entered interactively. `-write` only appends the generated profile to the existing config. If the existing config lacks remote credentials, existing config validation may reject the file; the user must update credentials manually.

## Testing

Tests should avoid real NocoDB instances.

Add unit tests for:

- Metadata client successful field decoding.
- Metadata client non-2xx error reporting.
- Metadata client rejection of malformed or empty field responses.
- Single-selection parsing.
- Multi-selection parsing, duplicate detection, and range validation.
- Profile generation from local and remote field lists.
- Rejection when selected sync fields are missing locally.
- Appending a profile to config while preserving unrelated config fields.
- Rejection of duplicate profile names in `-write` behavior.
- CLI-level behavior for print mode and write mode where practical.

The implementation should keep core generation logic independent from `os.Stdin` and `os.Stdout` so tests can use `strings.Reader` and `bytes.Buffer`.

## Acceptance Criteria

- Building `./cmd/noco-sync-profile-gen` succeeds.
- Running without `-write` prints one valid `sync_profiles` JSON object.
- Running with `-write` appends the generated profile to the target config file.
- Generated profiles pass the existing config validation rules.
- The tool lists fields from both local and remote NocoDB via Meta API.
- `sync_fields` can only include remote fields that also exist in the local table.
- Tokens entered interactively are not written to disk by the tool.
