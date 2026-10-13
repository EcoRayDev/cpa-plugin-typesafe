# TypeSafe for CLIProxyAPI

TypeSafe's typed judgment API as a native CLIProxyAPI plugin. Built by [EcoRay](https://github.com/EcoRayDev).

The plugin exposes native `POST /v1/systemone`, registers `jev-latest` for existing OpenAI-compatible clients, and supplies its own settings page. No separate proxy sidecar or management-dashboard fork.

**Host requirement:** the full integration requires [CLIProxyAPI PR #6589](https://github.com/router-for-me/CLIProxyAPI/pull/6589), which proposes RPC schema-7 native routes, HTTP safety and management model refresh. Stock 8.0.27 does not provide these capabilities. This project is not built-in support or an accepted official plugin-store listing. See [the architecture](docs/design.md).

## What Jev does

TypeSafe System One returns typed judgments from application state and questions. It is not a conversational model. Use the native endpoint for probabilities and structured decisions. The chat interface preserves compatibility for tools that can only select OpenAI-shaped models; its content is JSON answers, not invented conversational reasoning.

## Build

Requires Go 1.26 or newer, a C compiler and a CLIProxyAPI build with the native-route capability. The plugin uses the official native C ABI version 1, not Go's `plugin` build mode.

```sh
make test
make vet
make build
make package
```

The supported release targets are macOS and Linux, on ARM64 and x86-64. Libraries are `dist/typesafe.dylib` on macOS and `dist/typesafe.so` on Linux. Release ZIPs contain only the library at the ZIP root. `checksums.txt` uses SHA-256. Build with `-trimpath` so local source paths are not embedded in release artifacts. Windows releases are not provided: private-file ACL behavior has not been qualified.

If a Nix-provided `cc` is selected on macOS, use Apple's compiler explicitly: `CC=/usr/bin/clang make build` (and the same environment for `make test`, `make vet`, and `make package`).

## Install

1. Obtain a compatible host build. Until the upstream hook is merged, use [EcoRayDev's `feat/plugin-native-routes` branch](https://github.com/EcoRayDev/CLIProxyAPI/tree/feat/plugin-native-routes), not an unchanged stock host. Build it with `go build -trimpath -o cli-proxy-api ./cmd/server`; on macOS with Nix GCC selected, prefix the command with `CC=/usr/bin/clang`.
2. Build the library, or download the matching platform ZIP and verify it against the release's `checksums.txt` before extraction.
3. Place the library in the host's plugin directory.
4. Configure both the global plugin switch and this plugin's switch:

```yaml
plugins:
  enabled: true
  dir: "./plugins"
  configs:
    typesafe:
      enabled: true
      model: "jev-latest"
      credentials_file: "./private/typesafe/credentials.yaml"
```

The TypeSafe upstream key does **not** belong in this YAML. An example local-only host configuration with unmistakable placeholder client/management keys is in [examples/config.yaml](examples/config.yaml); replace them privately before use.

5. Open `/v0/resource/plugins/typesafe/settings` from the host's plugin resource menu. Authenticate with the **host management key**, save the TypeSafe key, choose the model and run Test connection. A blank replacement key preserves the saved value. Connection tests use saved settings, not an unsaved draft; Cancel clears the draft. Failed Save retains the existing credential and draft.

A private saved credential takes precedence over `TYPESAFE_API_KEY`; that environment variable is a bootstrap option, not an override for a key saved later. Supply secrets through a protected service environment or the settings form, not shell history. Keep all private state outside a source checkout.

## Native API

Use a normal CLIProxyAPI **client key**, not the management or TypeSafe key.

```http
POST /v1/systemone HTTP/1.1
Content-Type: application/json
Authorization: Bearer REPLACE_WITH_CLIENT_KEY

{
  "model": "jev-latest",
  "state": "The invoice was due yesterday and remains unpaid.",
  "questions": {
    "overdue": {
      "type": "noul",
      "instructions": "Is the invoice overdue?"
    }
  }
}
```

The native response retains TypeSafe's status and typed body. Rate-limit `Retry-After` is retained. Redirects are not followed and requests are not automatically retried. Disconnects cancel upstream work; upstream requests have a 120-second deadline and a 4 MiB response bound.

## OpenAI-compatible API

Select `jev-latest` in a client that discovers `/v1/models`. Send typed state/questions JSON as the last user message:

```json
{
  "model": "jev-latest",
  "messages": [
    {
      "role": "user",
      "content": "{\"state\":\"The invoice was due yesterday and remains unpaid.\",\"questions\":{\"overdue\":{\"type\":\"noul\",\"instructions\":\"Is the invoice overdue?\"}}}"
    }
  ]
}
```

Send this to `/v1/chat/completions`. The assistant content contains JSON answers. Plain text instead becomes a single Noul question, with prior conversation supplying state. Streaming formats the completed judgment as SSE chunks ending in `[DONE]`; TypeSafe itself does not stream generated tokens. Do not declare tool, vision or reasoning-generation support for this model.

The official model-router capability targets this plugin's own executor for its Jev aliases, without requiring a synthetic OAuth record or intercepting unrelated provider models. Saving a model pin updates `/v1/models` and chat routing immediately, without a restart. Native requests retain their explicit `model` field.

## Verified behavior

The shared library was loaded into a real schema-7 host on macOS ARM64 and exercised against TypeSafe's canonical API. Native, typed chat, plain Noul compatibility, and single-framed SSE returned genuine Jev judgments with reported usage; SSE ended with `finish_reason: stop` and `[DONE]`, without a trailing error. Provider validation status/body, client-versus-management authorization, request-body limits, disable/re-enable lifecycle, and the rendered settings workflow were exercised. Tests include decoded-secret redaction, private-file boundaries, and raw executor stream chunks.

No production sidecar or dashboard was replaced during these checks. CI is configured to build and test the other release targets; local macOS verification is not evidence for an unrun platform.

## Migration

Leave an existing sidecar and custom dashboard in place until the isolated plugin checks pass and a production cutover is explicitly approved. Preserve the host's client keys, OAuth directory and other provider configuration. Verify native Jev, chat, streaming, settings and downstream clients before removing obsolete sidecar routes. Do not restore stale credential snapshots over newer live state.

## Security and contributions

Read [SECURITY.md](SECURITY.md) before installing. Native plugins are trusted in-process code, not sandboxed extensions. Client, management and TypeSafe keys have distinct roles. The public settings page contains no credentials; its management API calls authenticate independently.

Source, history and release artifacts must be audited before publication. Do not attach production configs, OAuth files, keys or private infrastructure identifiers to issues. Contributions should include consumer-visible behavior checks and must keep the native typed protocol separate from chat compatibility.

MIT licensed. The native bridge follows Router-For.ME's official C ABI example; see [LICENSE](LICENSE) for attribution.
