# Native client and CNP v1

ciwi's native client is a separate Gio executable. It talks directly to the
server using CNP (ciwi Native Protocol), not HTTP, WebSockets, a browser engine,
or an embedded web view.

## Starting the endpoint and client

The native listener starts on UDP port 8113 by default, and the server
advertises it over mDNS. A normal server start is therefore enough for local
native clients to discover it:

```bash
ciwi server
```

Set `CIWI_NATIVE_ADDR` to another bind address to override the port, or to
`off` to disable the native listener.

Build and run the desktop client on macOS, Windows, or Linux:

```bash
go build -o ciwi-desktop ./cmd/ciwi-desktop
./ciwi-desktop -addr 127.0.0.1:8113
```

When `-addr` is omitted, the client discovers `_ciwi-native._udp` services over
mDNS and selects the first deterministic result. `CIWI_NATIVE_SERVER` supplies
the same explicit address as `-addr`. `-theme` or `CIWI_NATIVE_THEME` selects
one of the shared theme names.

The browser proof of the same declarative screen is available at
`/declarative-preview`; project navigation continues under
`/declarative-preview/projects/{projectId}` and execution navigation under
`/declarative-preview/jobs/{jobExecutionId}`. These routes are intentionally
separate from the established pages until behavioral parity is complete.

## Desktop packages

The `Build and release` chain publishes three native-client packages in
addition to the established server and agent binaries:

- a signed, notarized universal macOS DMG (`arm64` and `x86_64`);
- a Windows x64 WiX installer;
- a Linux amd64 ZIP with the executable, pixel-art icon, desktop entry, and
  installation notes.

All three derive their application icon from ciwi's pixel-art favicon. The
Linux build currently targets Gio's X11/OpenGL backend and deliberately omits
the optional Wayland and Vulkan backends to keep this first archive's native
dependency surface small. See the README inside the ZIP for runtime library
names.

## Wire contract

- Public schema: [`api/ciwi/native/v1/ciwi.proto`](../api/ciwi/native/v1/ciwi.proto)
- Public Go client: [`pkg/cnpclient`](../pkg/cnpclient)
- Transport: QUIC over UDP
- ALPN: `ciwi-native/1`
- Encoding: Protocol Buffers
- Framing: unsigned-varint byte length followed by one protobuf message
- Maximum control frame: 8 MiB
- Negotiation: the first bidirectional stream is `Hello` / `Welcome`
- Requests: one independent bidirectional stream per request
- Live state: a long-lived `WatchChanges` stream of coalescible invalidations
- Live output: a cursor-based `WatchJobOutput` stream of bounded event pages
- 0-RTT and QUIC datagrams: disabled in v1

The server exposes typed status codes rather than HTTP status codes. Mutating
commands carry an idempotency key. The server stores command receipts in SQLite
so a retry cannot enqueue the same pipeline twice. A pending receipt means the
original outcome is unknown and returns `UNAVAILABLE`; it is never guessed.

Regenerate the checked-in Go protobuf after changing the schema:

```bash
go run github.com/bufbuild/buf/cmd/buf@v1.57.2 generate
```

## Security boundary

CNP v1 intentionally retains ciwi's private-network/homelab trust model. QUIC
requires TLS 1.3, so traffic is encrypted, but v1 uses an ephemeral server
certificate and the client does not verify endpoint identity. There is no user
authentication or authorization boundary. Do not expose the endpoint to an
untrusted network.

This limitation is explicit in the public client and can later be replaced by
certificate pinning or a pairing flow without changing application services.

## Current scope

The vertical slices support server information, the shared front page, inline
queued/history execution cards with explicit job-detail navigation, project
details with nested pipelines/jobs/configured steps, job execution snapshots
with a horizontally navigable phase/step timeline, searchable and selectable
incremental output grouped into ciwi phases and YAML job steps, output copying
and tailing, ordinary and dry-run enqueue
commands for individual pipelines and named pipeline chains from the front page
and Project Details, including job-scoped execution controls, native back
navigation, and live invalidations. Output streams use
bounded cursor pages and clients cap their display buffer, so status refreshes
do not reload large logs. The front page can clear queued executions, flush all
terminal history, or delete one execution card through idempotent application
commands exposed by CNP; deleting history also removes its server-side
artifacts, but never clears agent caches or workspaces. Eligible Job Details
views can also queue an independent rerun or mark an active execution failed
through idempotent CNP commands. Cancellation updates server state but does not
forcibly terminate an agent process that is already running. Agents continue to
use the existing HTTP protocol.

Global Settings covers native-client appearance, connection context, project
management, agent administration, and server update/rollback controls. Theme
changes apply immediately and are stored in the user's config
directory as `ciwi/native-ui.json`, alongside keyed disclosure state; an explicit `-theme` or
`CIWI_NATIVE_THEME` value overrides the saved theme for that launch.

The Gio adapter resolves the same bundled logo, semantic icon names, theme
gradients, status tones, and badge roles used by the declarative browser proof;
these are renderer primitives rather than per-screen native drawings.
The desktop client deliberately has no HTTP fallback: missing CNP capabilities
fail visibly instead of silently coupling the native UI to browser endpoints.
