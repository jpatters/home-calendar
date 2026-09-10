# Noridoc: hottub

Path: @/internal/hottub

### Overview

- Driver for a Gecko in.touch2 hot tub WiFi module, spoken to directly over UDP on the LAN (port 10022 by default). It reads water temperature, the user's setpoint, the setpoint limits, and heater state, and it can write a new setpoint.
- Exposes two layers: package-level `Read` / `SetTarget` functions that open a short-lived `session` per call, and a `Fetcher` that polls on an interval, caches the last good `types.HotTubSnapshot`, serialises writes, and publishes every new reading through an `onUpdate` callback.
- The wire protocol is the same one used by the in.touch2 phone app and the geckolib Home Assistant integration; the byte offsets are hard-coded for a single spa pack (inYT config v65 / log v66) and any other pack is refused.

### How it fits into the larger codebase

- Follows the same fetcher shape as the other widget packages under @/internal (weather, tide, pool, ...): `New(onUpdate)`, `Start(ctx, cfg, interval)`, `Stop()`, `RefreshNow(ctx, cfg)`, `Snapshot()`. @/internal/server/router.go constructs the `Fetcher` with an `onUpdate` that broadcasts a `hottub` WebSocket frame via the Hub, and `applyFetcherConfig` starts or stops it whenever `types.HotTub.Enabled` changes.
- Unlike the read-only fetchers, this one also has `Fetcher.SetTarget`, which @/internal/server/handlers.go calls from `POST /api/hottub/target`. It publishes the post-write reading through the same `onUpdate` path, so a setpoint change reaches every connected display as an ordinary `hottub` frame with no special-case broadcast.
- Configuration comes from `types.HotTub` in @/internal/types/types.go (`Host` as `ip` or `ip:port`; the port is appended when missing) and the reading is `types.HotTubSnapshot`, which is the JSON shape the frontend consumes in @/web/src/types.ts.
- `ErrInvalidTarget` is the only sentinel error the package exports; the HTTP layer uses `errors.Is` on it to distinguish a client mistake (400) from a module failure (502).

```
 display (HotTubModal)                     in.touch2 module (UDP :10022)
      |  POST /api/hottub/target                     ^
      v                                              |  HELLO / SFILE / STATU / SPACK
 server.handleHotTubTarget --> hottub.Fetcher.SetTarget --> hottub.SetTarget --> session
                                       |
                                       v  publish -> onUpdate
                                Hub.Broadcast({type:"hottub"})
```

### Core Implementation

- `session` lifecycle (`dial`): open a UDP "connection", send `<HELLO>1</HELLO>` and take the spa ID from the `<HELLO>{spaID}|{name}</HELLO>` reply, then send `SFILE` and pass the `FILES,inYT_C65.xml,inYT_S66.xml` answer through `checkSpaPack`, which rejects anything other than config `inYT_C65` with log `inYT_S66`. Because both are pinned, the versions the write command must echo back are constants.
- Every non-HELLO payload is wrapped by `request` in a `<PACKT><SRCCN>client</SRCCN><DESCN>spaID</DESCN><DATAS>...</DATAS></PACKT>` envelope. `exchange` does the raw send/receive: it sets a deadline (the shorter of `requestTimeout` and the context deadline), writes once, and keeps reading datagrams until one *contains* the wanted marker, retrying the send `requestAttempts` times on timeout. Matching is on content rather than parsed XML because the module's replies do not close tags consistently, and unsolicited `STATP` pushes from the module are simply skipped by the inner read loop.
- Reading (`readSnapshot`) issues three `STATU` requests of `statusChunk` (20) bytes at offsets 0, 60 and 260 and assembles the snapshot from fixed positions: `SetpointG` at 1, `MinSetpointG`/`MaxSetpointG` at 66/68 (config struct), heater bits 5-6 of byte 260, `DisplayedTempG` at 277 and the `TempNotValid` bit at 279 (log struct). A set `TempNotValid` bit makes the read fail rather than return a bogus temperature. Raw temperatures are tenths of a degree above freezing: `toF(raw) = raw/10 + 32`, `fromF` is the inverse.
- Writing (`SetTarget` -> `session.writeWord`): fractional targets are rejected with `ErrInvalidTarget` before any network traffic; the session then reads only the limits chunk, rejects targets outside `[MinTargetF, MaxTargetF]` (also `ErrInvalidTarget`), sends `SPACK` + seq 192 + packType 10 (inYT) + length 7 + command 70 (SET_VALUE) + configVersion 65 + logVersion 66 + uint16 position + uint16 raw value, waits for `PACKS`, and finally re-reads so the caller gets what the tub actually reports. Commands use a sequence byte in the 192..255 range, mirroring the official app; a session sends at most one write so it is a constant.
- `Fetcher` state: `snapshot` behind an RWMutex (`Snapshot()` returns a copy, nil when nothing has been read or the fetcher is stopped); a separate `ioMu` taken by both the poll and `SetTarget`, so writes cannot interleave their datagrams and a poll that began before a write cannot publish the old setpoint after it; `cancel`/`doneWG` so `Stop()` blocks until the polling goroutine exits. `Start` always calls `Stop` first, so re-applying config never leaks a loop. `fetch` keeps the last good snapshot on error (UDP drops and the module's RF link are flaky) but clears it, and broadcasts nil, if `Host` is emptied.

### Things to Know

- Only inYT config v65 / log v66 offsets are valid. The constants at the top of `intouch2.go` were taken from geckolib's `inyt-cfg-65` and `inyt-log-66` definitions and verified against a real module on 2026-09-10; `checkSpaPack` exists precisely so a different pack fails loudly instead of decoding garbage.
- The module answers every `STATU` with sequence byte 0, so status replies cannot be matched to requests. After a resend, `exchange` briefly drains the socket so a late answer to the original request is not taken as the next chunk's reply and decoded as the wrong bytes.
- Reads are chunked at 20 bytes because the real module truncates `STATV` replies longer than roughly 39 bytes. A single large read of the 1024-byte status block does not work; the test fake in `intouch2_test.go` enforces the same cap so a regression to one big read fails the tests.
- `TargetF` is the config struct's `SetpointG` (position 1), not the log struct's `RealSetPointG` (position 275). `SetpointG` is the user's setpoint and the field that `SPACK` writes; `RealSetPointG` is read-only and can differ under economy mode.
- The status block's layout is: config struct at bytes 0..255, log struct from 256 onward. The three read offsets (0, 60, 260) are chosen so each 20-byte chunk covers the fields it needs; the `-60` / `-260` arithmetic in `readSnapshot` translates absolute positions into chunk-relative indices.
- Context cancellation must interrupt a blocking UDP read immediately: `dial` registers `context.AfterFunc` to slam the socket deadline to now. Without it, `Stop()` (and therefore a config save from the admin UI) would wait out a full request timeout.
- Tests never touch a real module. `intouch2_test.go` runs a loopback `fakeModule` seeded with a captured real config block and log block, and can be told to drop the first request, stay silent, emit unsolicited `STATP` pushes, or ignore `SPACK` writes to exercise the retry and error paths.

Created and maintained by Nori.
