# Security

## Trust boundary

This is a native shared-library plugin. CLIProxyAPI loads it into the host process; it is not sandboxed. Install only releases and source you trust. The plugin's private credential file protects against accidental disclosure, not a compromised host, administrator, or another malicious native plugin.

There are three separate credential roles:

- CLIProxyAPI client keys authorize model and native client requests.
- The CLIProxyAPI management key authorizes settings and connection tests.
- The TypeSafe upstream key is used only for requests to `https://api.typesafe.ai/v1/systemone`.

Never substitute one role for another. Keep management local or behind an authenticated private network. Do not expose the proxy to the public internet without a separate access-control design.

## Secret storage and responses

The upstream key does not belong in the host's plugin configuration, model catalog, resource page, example configuration, repository, build artifacts, or logs. `credentials_file` points to private plugin-owned state. Credential files are owner-readable/writable only; plugin-owned directories are owner-only. Settings responses reveal configured status and model, never the key. A blank replacement key preserves the saved value. Saving a replacement key is explicit.

`TYPESAFE_API_KEY` may bootstrap an installation without an existing saved credential. Prefer the authenticated settings page or a protected service environment over typing a secret into shell history. A native plugin with a public settings-page resource must still authorize every settings API request independently; loading the page is not authorization. The management key is held in page memory only, not browser persistent storage.

Provider errors and connection-test results must be redacted before display. The UI renders provider text as text, not HTML. Native forwarding preserves the typed API contract; errors and redirects must not disclose credentials. Redirect following and automatic retries are disabled.

Credential files are opened relative to an anchored directory descriptor, with final symlink following disabled and ownership/permissions validated on opened objects. Existing unsafe directories are rejected, not silently chmodded. Atomic same-directory replacement uses an exclusive 0600 file; reads and writes are bounded to 64 KiB. Provider redaction includes decoded JSON values, object keys and nested escaped error strings. Credential-bearing host HTTP callbacks suppress logging before raw upstream responses reach host logging sinks; responses are then sanitized for clients.

## Reporting

Do not include keys, OAuth files, private hostnames/IPs, production configs, or unredacted request logs in issues. Use GitHub's private vulnerability reporting for this repository when enabled. If that is unavailable, open an issue requesting a private reporting channel without publishing exploit details or credentials.

## Publication checks

Before releases, scan source and committed history with Gitleaks and inspect packaged shared libraries for credential values and private infrastructure identifiers. Check that ZIPs contain only the platform library at the archive root and that SHA-256 manifests match. These checks reduce accidental leaks; they do not prove absence of every possible secret or vulnerability.
