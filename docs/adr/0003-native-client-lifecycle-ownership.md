# ADR 0003: Native client lifecycle ownership

Status: accepted

## Context

On iOS, background suspension can interrupt a CNP request at any byte boundary.
The previous native adapter also allowed its controller goroutine to mutate Gio
renderer state directly. Together, those properties made a foreground frame
race with network teardown and allowed ordinary `EOF` errors from an expected
suspension to become user-visible notices.

Connection setup, retry timing, screen loads, output streams, downloads, and
operation recovery also had partially independent lifetimes. A result created
before suspension therefore needed a stronger rule than “ignore it if its
individual request was cancelled.”

## Decision

The native client uses the following ownership boundaries:

1. The Gio event goroutine is the sole owner of `Renderer` and the DOM runtime.
   The controller writes immutable commands to a one-way UI mailbox, which the
   event goroutine drains immediately before layout. Notice expiry is scheduled
   and consumed by renderer frames.
2. The controller remains the owner of navigation, operation presentation, and
   connection policy. A focused session supervisor owns the connection attempt,
   accepted transport, retry timer, backoff, session context, and monotonically
   increasing generation.
3. iOS focus delivery combines the latest focus level with an inactive epoch.
   This preserves a brief inactive edge even when inactive and active events are
   coalesced before the controller runs. Every inactive edge cancels active
   operations and tears down the entire session before a new generation starts.
4. Every session resource derives from the session context: CNP request streams,
   watches, screen loads, output streams, reconciliation, and artifact downloads.
   CNP cancellation interrupts reads, writes, and the logical stream. SSH
   cancellation closes the underlying connection while its context-free
   handshake APIs are blocked.
5. Query cancellation is terminal and quiet. A mutation cancelled before it can
   execute is also terminal; a mutation whose request may have reached the
   server becomes `outcome-unknown` and remains journaled.
6. Reconciliation performs receipt I/O in a read-only background inspection.
   The controller applies its plan only if the session generation is still
   current. The operation client broker stays empty until recovery completes,
   so no newly initiated remote action can overtake receipt verification.
7. Expected teardown errors (`context.Canceled`, `EOF`, and closed-transport
   equivalents) do not produce a snackbar. Policy failures such as unreadable
   SSH key material and host-key decisions pause connection attempts visibly.

This is a focused extraction rather than a full controller rewrite. It creates
testable lifecycle seams without moving navigation or presentation policy into
the transport layer. The CNP v1 protobuf and wire protocol are unchanged.

## Consequences

- A pre-background result can consume resources but cannot change the resumed
  session or renderer; stale successful sessions are explicitly closed.
- Passive screen refresh can begin after transport connection, while coordinated
  user actions and downloads wait for the recovery admission barrier.
- Safe mutation replay retains its original operation and idempotency identities.
  Receipt-only or server-mismatched entries are never guessed or replayed.
- The iOS build pipeline race-tests the client, operation coordinator, and Gio
  adapter before producing an archive.

## Physical-device release check

Simulator and race tests cannot model iOS suspension, radio transitions, or
Keychain behavior completely. Before TestFlight promotion, run this soak on a
physical iPhone against a reachable server:

1. Complete 20 short cycles: open a server-backed screen, lock for 5–15 seconds,
   unlock, return to Ciwi, and confirm automatic reconnection without an `EOF`
   notice, frozen UI, duplicate operation, or crash.
2. Complete three long cycles with the phone locked for at least five minutes.
   Include one cycle on Wi-Fi, one that changes network reachability while
   locked, and one from a Job Details output stream.
3. During one final short cycle, start a safe idempotent mutation immediately
   before locking. After return, confirm that receipt recovery either reports
   the known result or replays it once with the same idempotency key.
4. Capture the TestFlight crash report and device console if any cycle fails;
   do not promote that build until the failure is classified.
