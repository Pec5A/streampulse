# 0009 — Audio file upload and storage (ticket K2)

## Status
Accepted

## Context

K2 requires uploading audio files with "local storage, an interface ready for
S3, size/type validation", and a broadcaster interface on the mobile side.
Three questions had to be settled: where the bytes live, what can be trusted
in an upload request, and what "start a stream" means inside the app.

## Decisions

### A four-method `Storage` port, not a file path

`internal/domain/storage` defines `Save` / `Open` / `Delete` / `Exists`. No
signature leaks an `os.File` or a path: moving to S3 means writing a second
type that satisfies the interface, without touching a single use case.

`Open` returns an `io.ReadSeekCloser` rather than a plain `io.ReadCloser`.
That is not free: it is what lets the HTTP layer answer `Range` requests via
`http.ServeContent`, and therefore lets the mobile player **seek inside a
track** instead of re-downloading it from the start. It is the direct
counterpart of the seeking implemented in K1, which until now had no source to
apply to (a live stream has no past). An S3 implementation satisfies the same
contract with ranged GETs.

### Write to a temporary file then `rename`, never write in place

`Local.Save` writes to a temporary file, `fsync`s, then `rename`s — which is
atomic within a single filesystem. A connection dropped mid-upload therefore
leaves **no partial object**: a half-written file that a listener could stream
as if it were complete is worse than no file at all. Covered by
`TestLocal_FailedUploadLeavesNoObject`.

### The storage key is a server-generated UUID

The filename sent by the client is **never** used to build a path: it is kept
only for display and for the `Content-Disposition` header. The key is a UUID
plus the extension derived from the declared type.

`Local.resolve` additionally rejects, structurally, any key that is not a
plain single-segment name (no absolute path, no `/` or `\`, no `..`, no null
byte), then double-checks that the resolved path really is under the base
directory. Keys are server-generated today, but a security boundary must not
depend on the caller's good faith.

### Neither the declared type nor the declared size is trusted

**The declared type only selects the extension.** It is validated against a
list of accepted audio types, then the first 512 real bytes are sniffed.

But — and this is the non-obvious part — **we sniff to *reject*, not to
*allow***. `http.DetectContentType` only knows a handful of containers: an MP3
with no ID3 tag, an AAC or a FLAC all come back as
`application/octet-stream`. Allow-listing on the sniff would therefore reject
perfectly valid audio files.

**The first version, and why it was wrong.** Rejection initially relied on a
list of dangerous MIME types including `image/svg+xml`,
`application/javascript` and `application/xml`. The review of PR #23
(@JASSBR, widened by @SamyNikaia) showed that **`http.DetectContentType` never
emits those strings**. Verified against the real sniffer:

```
<svg> with no XML prolog -> text/plain; charset=utf-8
<svg> with an XML prolog -> text/xml; charset=utf-8    (caught by accident)
raw JavaScript           -> text/plain; charset=utf-8
XML with no prolog       -> text/plain; charset=utf-8
HTML with a doctype      -> text/html; charset=utf-8
```

Four of the eight entries were therefore dead strings, and a malicious SVG or
JS walked straight through. Chasing that with per-format signatures (`<svg`,
`<script`, …) is a race that is lost by default against every future text
payload.

**The rule we settled on** is structural and cannot be side-stepped by
dropping a prolog: **audio is binary, so audio never sniffs as text** — and
every active-content payload is text. A single check
(`strings.HasPrefix(sniffed, "text/")`) replaces the list.
`dangerousBinaryTypes` remains for the rare **non-text** types the sniffer
really does emit and that a browser would execute: `application/pdf`,
`application/postscript`, `application/x-shockwave-flash`.

Accepted residual limitation: an audio file under 512 bytes containing no byte
below 0x20 would be rejected. No real container produces that.

Defence in depth on the serving side: audio is returned with the type
validated at upload time (never a sniffed type), plus
`X-Content-Type-Options: nosniff` and a `default-src 'none'; sandbox` CSP.
Even if a hostile file got through, the browser would not execute it.

**Size is counted while streaming**, never read from a `Content-Length`: a
header is a claim, not a fact. Two barriers: `http.MaxBytesReader` cuts at the
transport level while the body arrives, and the use case counts the bytes
actually written.

### The row is the source of truth, the object follows

- If the database insert fails after the write, **the object is deleted**: an
  object with no row is garbage that nothing will ever reference again.
- On deletion, **the row goes first**: if the object delete then fails we are
  left with an orphaned file (recoverable, invisible to users) rather than a
  row pointing at nothing, which would break the whole catalogue listing.
- A row whose object has disappeared is served as **404, not 500**: that is
  the honest answer, it is not a server error.

### No public URL in the database

The reference repo stored a `file_url` in the database. That carves the host
name into the data: changing environment, or moving behind a CDN, would mean
rewriting rows. We store the opaque key and build `audio_url` at read time.
The key itself is never exposed in responses.

### Mobile: "broadcasting" means broadcasting a track, not capturing the mic

The ticket asks for "start/stop a stream, upload a track" — not microphone
capture. The broadcaster therefore picks one of their tracks and puts it on
air, like an internet radio station. Mic capture remains a natural evolution
(K4): it would plug into the same `BroadcastTransport`, changing only the
source.

**The stream is paced** (`pacedSource`). Without it, the file would go down
the socket as fast as the network accepts it: a four-minute track would be
over in two seconds, and a listener arriving a moment later would find nothing
left. The pacing is a deliberate approximation (16 KiB/s ≈ 128 kbps); real
pacing would read the bitrate from the container.

### Every platform dependency behind a port

`AudioFilePicker` in front of `file_picker`, `BroadcastTransport` in front of
the chunked POST — the same reasoning as `AudioEngine` in K1. The result: the
broadcaster's state machine, including "the audio source cannot be reached,
did we actually stop the broadcast?", is covered by unit tests with no device
and no server.

## Consequences

- One new environment variable: `STORAGE_PATH` (defaults to `./uploads`).
- Local storage assumes **a single node**: two replicas would not share their
  files. Same limitation as K1's stream registry, and the same decoupling
  point — this is exactly what the S3 implementation will solve, to be wired
  in K3 if the deployment moves to several replicas.
- `tracks.uploader_id` is `ON DELETE CASCADE`: the GDPR account deletion (Y2)
  removes the rows without Y2 having to know this table exists. **The stored
  objects are not removed by that cascade** — this will need either a periodic
  orphan sweep or an application hook in the account deletion. To be settled
  with Yassir once both our tickets are on `main`.
- Two new mobile dependencies: `file_picker`, `http_parser`.
- `internal/infrastructure/persistence/track_repository.go` has no unit tests
  (it needs a real Postgres) — the same limitation as the other repositories,
  already acknowledged in ADRs 0001 and 0008.
