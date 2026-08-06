# Native client and CNP v1

ciwi's native client is a separate Gio executable. It talks directly to the
server using CNP (ciwi Native Protocol), not HTTP, WebSockets, a browser engine,
or an embedded web view.

## Starting the endpoint and client

The native listeners start on UDP and TCP port 8113 by default, and the server
advertises both over mDNS. A normal server start is therefore enough for local
native clients to discover them:

```bash
ciwi server
```

Set `CIWI_NATIVE_ADDR` to another bind address to override both transports, or
to `off` to disable both. `CIWI_NATIVE_QUIC_ADDR` and
`CIWI_NATIVE_TCP_ADDR` can independently override or disable one transport.

Build and run the desktop client on macOS, Windows, or Linux:

```bash
go build -o ciwi-desktop ./cmd/ciwi-desktop
./ciwi-desktop -addr tcp://127.0.0.1:8113
```

When `-addr` is omitted, the client discovers `_ciwi-native._udp` and
`_ciwi-native._tcp` services over mDNS, preferring QUIC when both describe the
same server and falling back to another discovered endpoint when the preferred
route cannot be reached. `CIWI_NATIVE_SERVER` supplies the same explicit endpoint as
`-addr`. A bare `host:port` remains a QUIC endpoint for compatibility;
`quic://host:port` and `tcp://host:port` are unambiguous. The connection screen
and Global Settings can persist either automatic discovery or an explicit
endpoint in the native client's local preferences. Command-line and environment
addresses take precedence at startup. `-theme` or `CIWI_NATIVE_THEME`
selects one of the shared theme names. `-route` or `CIWI_NATIVE_ROUTE` selects
an initial route such as `/projects/1`, `/agents`, or `/settings`.

TCP makes an ordinary OpenSSH local forward sufficient for remote access:

```bash
ssh -N -L 8113:10.77.77.2:8113 user@jumphost
ciwi-desktop -addr tcp://127.0.0.1:8113
```

The server-side TCP listener must be reachable from the SSH host. No UDP tunnel,
TUN interface, HTTP fallback, or client-machine routing setup is required.

### Built-in SSH mode

Global Settings also provides an SSH connection mode that opens the CNP TCP
stream directly through a jump host, without creating a localhost listener.
Configure the jump-host address and username plus the server-side CNP
destination (for example `10.77.77.2:8113`). The client can generate a
device-specific Ed25519 key and copy a restricted `authorized_keys` entry that
permits forwarding only to that destination.

The first connection pauses at the presented SSH host-key fingerprint. Verify
it through an independent trusted channel before selecting **Trust This Host
Key**. Reconnects remain paused for an unknown or changed host key. Apple builds
store the device private key in Keychain; Windows and Linux store it beside the
native preferences with user-only permissions. See [`files.md`](files.md).

The browser proof of the same declarative screen is available at
`/declarative-preview`; project navigation continues under
`/declarative-preview/projects/{projectId}` and execution navigation under
`/declarative-preview/jobs/{jobExecutionId}`. These routes are intentionally
separate from the established pages until behavioral parity is complete.

## Native client packages

The `Build and release` chain publishes three native-client packages in
addition to the established server and agent binaries, and uploads the iOS
client to TestFlight:

- a signed, notarized universal macOS DMG (`arm64` and `x86_64`);
- a Windows x64 WiX installer;
- a Linux amd64 ZIP with the executable, pixel-art icon, desktop entry, and
  installation notes;
- a signed iPhone and iPad archive submitted to App Store Connect.

All clients derive their application icon from ciwi's pixel-art favicon. The
Linux build currently targets Gio's X11/OpenGL backend and deliberately omits
the optional Wayland and Vulkan backends to keep this first archive's native
dependency surface small. See the README inside the ZIP for runtime library
names.

The focused `Build and publish iOS client` chain builds the Gio
client as an arm64 static framework, links it into the minimal UIKit host in
`packaging/ios`, archives the app with automatic Xcode signing, validates the
archive, and uploads it to TestFlight. Compact overrides provide phone-width
layouts, full-screen disclosure sheets, touch scrolling, and compact job-detail
navigation while retaining the same screen definitions used on desktop.

The macOS build agent must have Xcode signed into the Apple developer account
for team `KFBA7Q5H76`, automatic signing access, and permission to upload the
`nl.izmar.ciwi` application. The App Store Connect application record is a
one-time manual prerequisite. A successful chain means App Store Connect
accepted the upload; processing completion and tester assignment remain in App
Store Connect.

## Wire contract

- Public schema: [`api/ciwi/native/v1/ciwi.proto`](../api/ciwi/native/v1/ciwi.proto)
- Public Go client: [`pkg/cnpclient`](../pkg/cnpclient)
- Transports: QUIC over UDP, or TLS over TCP with Yamux streams
- ALPN: `ciwi-native/1`
- Encoding: Protocol Buffers
- Framing: unsigned-varint byte length followed by one protobuf message
- Maximum control frame: 8 MiB
- Negotiation: the first bidirectional stream is `Hello` / `Welcome`
- Requests: one independent bidirectional stream per request; QUIC supplies
  streams directly and Yamux supplies the same abstraction over TCP
- Live state: a long-lived `WatchChanges` stream of coalescible invalidations
- Live output: a cursor-based `WatchJobOutput` stream of bounded event pages
- 0-RTT and QUIC datagrams: disabled in v1

The transport-neutral CNP handler owns the hello exchange, framing, request
dispatch, live watches, and application/presentation mappings. QUIC and TCP
adapters only establish sessions and expose logical streams. Yamux is confined
to the internal TCP stream adapter and is not part of CNP's public API. Adding
another multiplexed transport therefore does not duplicate CNP operations or
leak transport choices into application services.

The server exposes typed status codes rather than HTTP status codes. Mutating
commands carry an idempotency key. The server stores command receipts in SQLite
so a retry cannot enqueue the same pipeline twice. A pending receipt means the
original outcome is unknown and returns `UNAVAILABLE`; it is never guessed.

Regenerate the checked-in Go protobuf after changing the schema:

```bash
go run github.com/bufbuild/buf/cmd/buf@v1.57.2 generate
```

## Security boundary

CNP v1 intentionally retains ciwi's private-network/homelab trust model. Both
QUIC and TCP require TLS 1.3, so traffic is encrypted, but v1 uses an ephemeral
server certificate and the client does not verify endpoint identity. There is
no user authentication or authorization boundary. Do not expose either
endpoint directly to an untrusted network. An SSH local forward protects and
authenticates the outer route, but the client still does not independently
verify that the CNP process behind the forwarded port is ciwi.

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
navigation, managed YAML creation/editing, and live invalidations. Output streams use
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
management, agent administration, and server update/rollback controls. Agent
details can authorize, activate, refresh tools, restart, update, wipe caches,
flush per-agent history, delete snapshots, and queue ad-hoc scripts in any shell
the agent advertises. Mutations and queries run asynchronously, expose busy
state, and report short-lived success/error notices without blocking navigation.

Theme changes apply immediately and are stored in the user's config directory
as `ciwi/native-ui.json`, alongside disclosure and graph/list state, connection
settings, the trusted SSH fingerprint, and the last successful discovered
endpoint. An explicit `-theme` or `CIWI_NATIVE_THEME` value overrides the saved
theme for that launch.

The Gio adapter resolves the same bundled logo, semantic icon names, theme
gradients, status tones, and badge roles used by the declarative browser proof;
these are renderer primitives rather than per-screen native drawings.
The native client deliberately has no HTTP fallback: missing CNP capabilities
fail visibly instead of silently coupling the native UI to browser endpoints.
