# TypeSafe plugin architecture

## Objective

Provide TypeSafe's System One judgment API through CLIProxyAPI without a separate HTTP sidecar or a modified management dashboard. Preserve the native typed request protocol and an explicitly limited OpenAI chat compatibility interface for existing clients.

## Host capability

The proposed CLIProxyAPI `NativeAPI` capability registers exact client routes under `/v1`. The host retains responsibility for client-key authentication, request-body limits, route ownership, plugin enable/disable/reload, cancellation and panic isolation. Built-in and higher-priority routes win. Plugins cannot register management paths, wildcard routes or traversal paths through this capability.

The native C ABI stays at version 1. RPC schema 7 adds `native.register`, `native.handle` and the `native_api` capability. Native handlers receive the request method, path, query, body and non-credential headers. Authorization and API-key headers are not delivered to the native plugin handler. A scoped host callback retains the originating request context.

Optional host HTTP callback fields disable redirects, bound response bytes, set a timeout and suppress sensitive callback logging without changing existing plugins' defaults. This plugin requests no redirects, a 4 MiB response limit, a 120-second upstream deadline and suppression before raw upstream values enter log sinks. Older hosts must not silently ignore these safeguards: the complete plugin requires the proposed schema-7 host support.

## Plugin responsibilities

- Model catalog: `jev-latest` and the configured model, with honest judgment-oriented capability metadata.
- Model routing: the official `model_router` capability directs only the registered Jev aliases to this plugin's own static executor before OAuth selection. No synthetic host credential is needed.
- Native route: `POST /v1/systemone`, preserving TypeSafe's `state`, `model` and `questions` request and its response status/body.
- Chat compatibility: typed JSON in the last user message passes through; plain text becomes a single Noul question. Earlier conversation supplies state. The reply contains JSON answers, not fabricated conversational reasoning.
- Streaming compatibility: emit separate raw OpenAI JSON answer and terminal stop/usage chunks. The host owns SSE framing and `[DONE]`. This does not turn TypeSafe into a token-streaming upstream.
- Hot model changes: successful settings responses request host model and executor/catalog re-registration; old pins disappear and new pins become discoverable without a reload.
- Authenticated management: read configured status/model, replace or preserve the upstream key, and test the connection.
- Public resource: an embedded settings page with no embedded credentials or saved browser keys. Every management API action authenticates independently.

## Credentials

Client, management and upstream keys have distinct roles. Only the host authenticates client and management requests. The TypeSafe key remains in private plugin-owned state or a protected bootstrap environment. Saving a replacement key atomically persists owner-only state and makes it effective immediately. Blank replacement input preserves the existing key; saved credentials take precedence over bootstrap environment values.

A native shared-library plugin runs inside the host process and is trusted code, not a sandbox. The credentials policy prevents accidental disclosure but does not protect against a compromised host or malicious sibling plugin.

## Acceptance

Verify the real host loading the shared library, not just calls into plugin functions. Cover model discovery, native requests, chat and terminal SSE completion; unauthorized keys, body limits, reserved-route collisions, plugin disable/reload, cancellation, provider errors and redaction; management key retention, settings persistence and actual rendered UI behavior. Publish only audited source/history and packaged libraries. Production migration is a separate approval and must preserve clients' existing endpoints and credentials.
