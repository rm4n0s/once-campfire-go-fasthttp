# once-campfire-go

Campfire in Go, ported from [ONCE Campfire in Rust](https://github.com/basecamp/once-campfire-rust).
It uses the Go standard library for routing, templates, SQL access, cryptography and process
lifecycle, and [fasthttp](https://github.com/valyala/fasthttp) for HTTP serving. There is no web framework, ORM, dependency injection container or frontend framework added
by the port. The original Turbo/Stimulus/Lexxy frontend is retained.

The application includes setup and invitations, sign-in and session transfers, account and user
administration, open/closed/direct rooms, messages and attachments, editing/deletion, boosts,
mentions, search, notifications, bot APIs/webhooks, link previews, Web Push, and Action Cable
presence, typing, read/unread and live updates. It includes media processing, the public HTTP/TLS
front server, automatic certificates, production packaging and backup/restore hooks.

SQLite schema, storage keys/layout, password hashes and Rails signed/encrypted cookies are preserved.
An upgrade test checks Go and Rust cookies in both directions and Rust reading/searching Go-written
messages. The port is functionally implemented; **strict HTML/network parity is not complete**.
See [validation](plans/validation.md) and the known differences below before replacing an installation.
The pinned Rust source is in `reference/`, with its Rails source in `reference/reference/`.

## Dependencies

- Go 1.27.1: `html/template`, `database/sql`, `crypto`, `encoding/json`, `embed`, `testing`.
- `github.com/valyala/fasthttp`: HTTP/1.1 server for the application and public listeners, wrapped by
  `internal/httpx`, which keeps the net/http handler shape (`ResponseWriter`, `Request`, middleware).
  `net/http` remains only for value types and pure helpers that `httpx` re-exports (cookies, status
  text, `ServeContent`'s range logic), the outbound clients (webhooks, link previews, Web Push) and
  tests that act as clients.
- `github.com/mattn/go-sqlite3`: SQLite with FTS5, through CGO.
- `github.com/coder/websocket`: WebSocket transport; the Action Cable protocol and channels are local code.
- `golang.org/x/crypto`: bcrypt and ACME. The pinned revision includes the upstream ACME missing-Location fix.
- `golang.org/x/net`: HTML tokenization; a local tree-builder fork matches the reference parser.
- `golang.org/x/text`: indirect Unicode support.
- Native libvips, ffmpeg/ffprobe and libzstd: image/video processing and public HTTP zstd compression.

A local build needs a C compiler, pkg-config, libvips and libzstd development headers, ffmpeg, and
Python 3 for asset generation. Assets are built from the pinned sources without Ruby, Rust or Node.
The Dockerfile pins the reference media toolchain: libvips 8.16.1 and ffmpeg 7.1.5.

## Run

```sh
git submodule update --init --recursive
bin/build
export SECRET_KEY_BASE="$(openssl rand -hex 64)"
DISABLE_SSL=1 HTTP_PORT=8080 ./campfire server
```

Open <http://localhost:8080/first_run>. Keep the same `SECRET_KEY_BASE` across restarts; use an
existing installation's secret to retain logins when migrating its database and files.

```sh
docker build -t once-campfire-go .
docker run --rm -p 8080:80 -e DISABLE_SSL=1 -e SECRET_KEY_BASE \
  -v campfire-go-storage:/rails/storage once-campfire-go
```

The container runs as uid/gid 1000. For a bind mount, make its storage writable by that user.
`CAMPFIRE_STORAGE_PATH` defaults to `storage`, with databases in `db/`, media in `files/`, backups
in `backups/`, and certificate cache in `thruster/`. `CAMPFIRE_DATABASE_PATH` and `CAMPFIRE_FILES_PATH`
override individual locations; `RAILS_ENV` defaults to `production`. `campfire db:prepare` initializes
an empty database and checks migration versions; existing databases missing migrations are rejected.

The public listener uses `HTTP_PORT=80`. Set `TLS_DOMAIN` for automatic ACME certificates and HTTPS
on `HTTPS_PORT=443`. The internal application listener defaults to `TARGET_BIND=127.0.0.1` and
`TARGET_PORT=3000`. The front server provides HTTP/1.1 (no HTTP/2 or H2C; see known differences), gzip/zstd with compression jitter,
a bounded response cache and graceful shutdown. `campfire backup` writes an atomic SQLite snapshot;
the image provides ONCE's `/hooks/pre-backup` and `/hooks/post-restore` hooks.

## Validation

```sh
bin/check                              # gofmt, assets, vet, all tests with the race detector
bin/check-assets                       # asset digests/importmap
bin/check-upgrade --rust-root ../once-campfire-rust
node bin/check-browser.mjs
bin/check-screens --out .cache/screens --only '**'
bin/check-acme
bin/check-container --image once-campfire-go:verification
```

The browser/upgrade tools use the Rust checkout's parity seed and installed Playwright. Screen
comparison runs both binaries on disposable copies of each seed, recording their hashes and keeping
the original parity masks. Reports distinguish screenshots, accessibility, server/live DOM, network
responses and Cable traffic. There are no Go-specific allowlists hiding failures.

Package tests use temporary databases and do not silently skip integration tests for a missing seed.
They include Rails signing/encryption and serialization vectors, 658 rich-text cases, 385 user-agent
cases, QR codes, route recognition, storage/ranges, transactions, access control, Cable, jobs,
90 Open Graph cases, 19 webhook cases and Web Push encryption/local delivery. Media metadata tests
run locally; byte-for-byte output tests require the pinned container toolchain:

```sh
docker build --target toolchain -t once-campfire-go:toolchain .
mkdir -p .cache/tmp .cache/docker-go-build .cache/docker-go-mod
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/src" -w /src \
  -e GOCACHE=/src/.cache/docker-go-build -e GOMODCACHE=/src/.cache/docker-go-mod \
  -e TMPDIR=/src/.cache/tmp once-campfire-go:toolchain \
  go test -tags 'sqlite_fts5 media_vectors' ./internal/storage
```

`bin/build` and `bin/check` use mise when Go is absent from PATH and keep temporary build files in
`.cache/`. Live ACME is tested against a local Pebble CA, including restart with the CA offline.
Container verification exercises setup, a live SQLite backup, offline restore, and restart.

## Benchmarks

### fasthttp vs net/http vs Rust (local run, 2026-10-08)

The Go server was moved from `net/http` to fasthttp and measured against the previous `net/http`
build of the same application and against the pinned Rust implementation, with the repository's own
harness (`bench/application`) and the Rust repository's load generator. Raw samples, metadata, binary
and source hashes, server logs and response bodies are in
[`bench/results/fasthttp-20261008/`](bench/results/fasthttp-20261008/); its
[`report.md`](bench/results/fasthttp-20261008/report.md) is the harness output that the tables below reproduce.

**Method.** Three release binaries run the same parity seed (a fresh copy per run) on the application
HTTP/1.1 listener with identity encoding: Rust (`reference/`, pinned commit `64f8635`), Go `net/http` (the
repository at `a6c1359`, built before any change) and Go fasthttp (this tree, uncommitted at measurement;
per-file source hashes are in `metadata.json`). Both Go binaries use `-tags sqlite_fts5 -trimpath`, Go 1.27.1 and
the same libvips. Server on CPUs 8-11 (`GOMAXPROCS=4`), load generator on CPUs 12-15, 2 s warmup then 5 s
samples, 3 repetitions with the application order rotated each repetition (never two applications at once),
1/16/64 HTTP clients, 100/1,000/10,000 Action Cable clients with and without permessage-deflate, and 5
upload+thumbnail samples. Every response is validated (message and room IDs, avatar and CSS bytes), each
acknowledged write must be persisted and in the FTS index, and every Cable client must receive every message.
Result: 0 HTTP errors, 0 incomplete deliveries, 694,153 verified posts, identical thumbnail bytes.

**Machine.** AMD Ryzen AI MAX+ 395, 32 logical CPUs, Linux 7.2.9, `powersave` governor, a desktop session
running (load average 1.47 before the run), libvips 8.18.3 (unpacked from the Fedora packages; the
container toolchain pins 8.16.1), rustc 1.98.1.

#### Summary (median requests/s)

| Workload, 16 clients (requests/s) | Rust | Go `net/http` | Go fasthttp | fasthttp vs `net/http` | fasthttp vs Rust |
|---|---:|---:|---:|---:|---:|
| Room page | 28,510 | 34,890 | 41,039 | 1.18× | 1.44× |
| Messages page | 32,760 | 36,631 | 42,338 | 1.16× | 1.29× |
| Sidebar | 37,720 | 67,386 | 85,922 | 1.28× | 2.28× |
| Search | 30,758 | 54,359 | 64,340 | 1.18× | 2.09× |
| Avatar | 54,015 | 33,677 | 39,830 | 1.18× | 0.74× |
| Static CSS | 511,106 | 313,968 | 379,080 | 1.21× | 0.74× |
| Post a message | 6,739 | 4,342 | 4,318 | 0.99× | 0.64× |

| Workload, 1 clients (requests/s) | Rust | Go `net/http` | Go fasthttp | fasthttp vs `net/http` | fasthttp vs Rust |
|---|---:|---:|---:|---:|---:|
| Room page | 5,085 | 4,516 | 7,409 | 1.64× | 1.46× |
| Messages page | 6,775 | 4,648 | 7,413 | 1.59× | 1.09× |
| Sidebar | 9,114 | 16,386 | 20,669 | 1.26× | 2.27× |
| Search | 9,078 | 10,667 | 13,482 | 1.26× | 1.49× |
| Avatar | 15,033 | 8,165 | 9,989 | 1.22× | 0.66× |
| Static CSS | 79,926 | 63,023 | 75,893 | 1.20× | 0.95× |
| Post a message | 4,043 | 2,098 | 2,180 | 1.04× | 0.54× |

| Workload, 64 clients (requests/s) | Rust | Go `net/http` | Go fasthttp | fasthttp vs `net/http` | fasthttp vs Rust |
|---|---:|---:|---:|---:|---:|
| Room page | 28,915 | 35,661 | 36,392 | 1.02× | 1.26× |
| Messages page | 32,664 | 37,144 | 38,145 | 1.03× | 1.17× |
| Sidebar | 38,844 | 68,836 | 88,035 | 1.28× | 2.27× |
| Search | 35,044 | 53,956 | 63,454 | 1.18× | 1.81× |
| Avatar | 55,862 | 35,500 | 40,405 | 1.14× | 0.72× |
| Static CSS | 525,219 | 336,993 | 423,527 | 1.26× | 0.81× |
| Post a message | 9,294 | 4,741 | 4,790 | 1.01× | 0.52× |

- fasthttp is **16-28% faster than `net/http` at 16 clients** on every read workload, and uses 11-25% less
  CPU per request (see the CPU µs/request column below). At 64 clients the gain on the large HTML pages
  shrinks to 2-3% because SQLite, not the HTTP layer, is the bottleneck there; the small responses keep
  their lead (sidebar +28%, search +18%, avatar +14%, static CSS +26%).
- Against Rust, Go (either server) leads on the four cached HTML pages and trails on avatar (0.7×) and static
  files (0.7×) and on writes: posting a message is bound by the SQLite write path, where fasthttp does not
  help (0.99×) and Rust is 1.6-2.0× faster.
- Action Cable throughput is unchanged within noise (100 clients: 2,102 vs 2,083 messages/s; deflated 2,398
  vs 2,318). Rust is 1.2-2.5× faster across the Cable configurations.
- **Memory is worse with fasthttp.** After the whole workload sequence the process holds 2,140 MiB PSS, against
  1,082 MiB for `net/http` and 405 MiB for Rust. After the HTTP suite alone the two Go builds are close
  (141 vs 130 MiB), so the growth comes during the Cable suite with up to 10,000 sockets. Per-socket buffers
  (the 64 KiB read buffer and request context fasthttp keeps for each upgraded connection) are the likely
  cause; it was not isolated or tuned. At 10,000 non-deflated clients fasthttp also had worse tail latency
  (all-clients p99 1,025 ms vs 632 ms) for slightly higher throughput.
- The post_message 64-client minimum (448/554 req/s) is a single stalled sample in one repetition; it appears
  for Rust and both Go builds, and the medians are unaffected.

#### HTTP

Rates show median (minimum–maximum); latency is the median of each run’s percentile.

| Workload | Clients | App | Requests/s (range) | p50 ms | p90 ms | p99 ms | CPU µs/request |
|---|---:|---|---:|---:|---:|---:|---:|
| room_show | 1 | rust | 5,085 (4,806–5,778) | 0.183 | 0.207 | 0.310 | 131.0 |
| room_show | 1 | go | 4,516 (4,509–4,638) | 0.219 | 0.231 | 0.273 | 130.2 |
| room_show | 1 | go-fasthttp | 7,409 (5,720–9,508) | 0.127 | 0.160 | 0.263 | 83.6 |
| room_show | 16 | rust | 28,510 (28,500–28,794) | 0.537 | 0.774 | 1.039 | 122.0 |
| room_show | 16 | go | 34,890 (34,762–35,217) | 0.334 | 0.889 | 2.009 | 94.3 |
| room_show | 16 | go-fasthttp | 41,039 (40,367–41,207) | 0.316 | 0.726 | 1.400 | 83.1 |
| room_show | 64 | rust | 28,915 (28,173–28,919) | 2.153 | 3.057 | 4.027 | 121.5 |
| room_show | 64 | go | 35,661 (35,643–36,088) | 1.494 | 3.401 | 6.311 | 95.1 |
| room_show | 64 | go-fasthttp | 36,392 (36,202–36,567) | 1.399 | 3.667 | 6.455 | 98.0 |
| messages_page | 1 | rust | 6,775 (6,286–7,428) | 0.130 | 0.177 | 0.242 | 105.9 |
| messages_page | 1 | go | 4,648 (4,590–6,292) | 0.213 | 0.222 | 0.255 | 127.2 |
| messages_page | 1 | go-fasthttp | 7,413 (5,911–8,832) | 0.126 | 0.155 | 0.225 | 80.9 |
| messages_page | 16 | rust | 32,760 (32,622–32,887) | 0.473 | 0.650 | 0.836 | 106.5 |
| messages_page | 16 | go | 36,631 (36,539–36,926) | 0.320 | 0.844 | 1.876 | 91.0 |
| messages_page | 16 | go-fasthttp | 42,338 (42,331–42,700) | 0.310 | 0.690 | 1.314 | 81.1 |
| messages_page | 64 | rust | 32,664 (32,250–32,895) | 1.913 | 2.637 | 3.379 | 106.6 |
| messages_page | 64 | go | 37,144 (37,090–37,262) | 1.453 | 3.227 | 5.903 | 92.1 |
| messages_page | 64 | go-fasthttp | 38,145 (37,988–38,399) | 1.334 | 3.509 | 6.079 | 93.1 |
| sidebar | 1 | rust | 9,114 (8,349–9,114) | 0.106 | 0.114 | 0.129 | 106.2 |
| sidebar | 1 | go | 16,386 (16,386–16,681) | 0.058 | 0.066 | 0.112 | 60.8 |
| sidebar | 1 | go-fasthttp | 20,669 (20,562–20,878) | 0.045 | 0.047 | 0.123 | 45.1 |
| sidebar | 16 | rust | 37,720 (37,365–37,851) | 0.410 | 0.590 | 0.765 | 99.8 |
| sidebar | 16 | go | 67,386 (67,298–68,348) | 0.161 | 0.498 | 1.067 | 53.3 |
| sidebar | 16 | go-fasthttp | 85,922 (85,661–86,384) | 0.133 | 0.375 | 0.818 | 41.6 |
| sidebar | 64 | rust | 38,844 (38,299–38,893) | 1.603 | 2.297 | 2.975 | 97.0 |
| sidebar | 64 | go | 68,836 (68,559–69,343) | 0.495 | 2.351 | 5.003 | 53.2 |
| sidebar | 64 | go-fasthttp | 88,035 (87,745–88,650) | 0.305 | 1.930 | 4.543 | 41.9 |
| search | 1 | rust | 9,078 (8,278–9,384) | 0.100 | 0.132 | 0.153 | 102.2 |
| search | 1 | go | 10,667 (10,012–10,835) | 0.088 | 0.112 | 0.146 | 90.6 |
| search | 1 | go-fasthttp | 13,482 (13,198–13,527) | 0.066 | 0.082 | 0.154 | 56.0 |
| search | 16 | rust | 30,758 (30,504–30,831) | 0.468 | 0.782 | 1.126 | 96.8 |
| search | 16 | go | 54,359 (54,281–54,677) | 0.208 | 0.583 | 1.292 | 63.8 |
| search | 16 | go-fasthttp | 64,340 (63,661–64,502) | 0.206 | 0.440 | 0.910 | 55.3 |
| search | 64 | rust | 35,044 (34,930–35,251) | 1.674 | 2.515 | 4.107 | 89.3 |
| search | 64 | go | 53,956 (53,770–54,236) | 0.854 | 2.615 | 5.167 | 65.7 |
| search | 64 | go-fasthttp | 63,454 (63,187–63,546) | 0.777 | 2.115 | 4.017 | 57.1 |
| avatar | 1 | rust | 15,033 (14,704–15,498) | 0.066 | 0.074 | 0.083 | 68.0 |
| avatar | 1 | go | 8,165 (8,140–8,166) | 0.119 | 0.129 | 0.193 | 129.3 |
| avatar | 1 | go-fasthttp | 9,989 (9,988–10,058) | 0.093 | 0.100 | 0.279 | 100.3 |
| avatar | 16 | rust | 54,015 (53,970–54,085) | 0.283 | 0.409 | 0.612 | 65.2 |
| avatar | 16 | go | 33,677 (33,574–33,793) | 0.339 | 0.924 | 1.932 | 112.3 |
| avatar | 16 | go-fasthttp | 39,830 (39,432–39,931) | 0.329 | 0.787 | 1.394 | 95.3 |
| avatar | 64 | rust | 55,862 (55,575–55,900) | 1.108 | 1.579 | 2.135 | 62.5 |
| avatar | 64 | go | 35,500 (35,339–35,557) | 1.663 | 2.889 | 4.323 | 107.1 |
| avatar | 64 | go-fasthttp | 40,405 (39,858–40,488) | 1.216 | 3.453 | 5.911 | 94.6 |
| static_css | 1 | rust | 79,926 (77,322–80,357) | 0.012 | 0.013 | 0.014 | 7.7 |
| static_css | 1 | go | 63,023 (62,908–63,099) | 0.014 | 0.017 | 0.032 | 11.8 |
| static_css | 1 | go-fasthttp | 75,893 (75,736–78,064) | 0.012 | 0.013 | 0.029 | 8.6 |
| static_css | 16 | rust | 511,106 (503,874–511,226) | 0.030 | 0.040 | 0.085 | 6.2 |
| static_css | 16 | go | 313,968 (313,658–317,460) | 0.039 | 0.087 | 0.173 | 9.5 |
| static_css | 16 | go-fasthttp | 379,080 (372,081–379,461) | 0.032 | 0.084 | 0.139 | 7.1 |
| static_css | 64 | rust | 525,219 (521,986–528,604) | 0.114 | 0.177 | 0.347 | 5.9 |
| static_css | 64 | go | 336,993 (336,286–338,196) | 0.138 | 0.374 | 0.901 | 9.4 |
| static_css | 64 | go-fasthttp | 423,527 (419,649–433,563) | 0.109 | 0.314 | 0.694 | 7.0 |
| post_message | 1 | rust | 4,043 (4,011–4,067) | 0.234 | 0.279 | 0.347 | 278.5 |
| post_message | 1 | go | 2,098 (2,065–2,549) | 0.427 | 0.499 | 2.891 | 474.7 |
| post_message | 1 | go-fasthttp | 2,180 (2,153–2,181) | 0.396 | 0.490 | 2.983 | 424.6 |
| post_message | 16 | rust | 6,739 (6,095–7,332) | 1.631 | 2.191 | 5.423 | 305.9 |
| post_message | 16 | go | 4,342 (4,314–4,798) | 2.437 | 8.351 | 15.391 | 406.5 |
| post_message | 16 | go-fasthttp | 4,318 (4,288–4,415) | 2.423 | 8.471 | 15.335 | 385.9 |
| post_message | 64 | rust | 9,294 (448–9,477) | 6.567 | 9.271 | 12.167 | 296.0 |
| post_message | 64 | go | 4,741 (554–4,789) | 9.935 | 30.511 | 61.919 | 360.3 |
| post_message | 64 | go-fasthttp | 4,790 (4,789–4,857) | 9.799 | 30.223 | 59.647 | 324.4 |

#### Action Cable

Throughput counts posted messages delivered to **all** clients; one such message produces one frame per client. Latency includes delivery, rather than just accepting a post. The reference load generator reports p90 rather than p95.

| Clients | Deflate | App | Messages/s (range) | Frames/s | Per-client p99 ms | All-clients p99 ms | Wire MB/s |
|---:|---|---|---:|---:|---:|---:|---:|
| 100 | False | rust | 2,537.6 (337.8–4,369.5) | 253,757 | 3.361 | 4.663 | not measured |
| 100 | False | go | 2,083.3 (52.6–2,084.3) | 208,325 | 6.495 | 7.783 | not measured |
| 100 | False | go-fasthttp | 2,101.9 (2,081.8–2,112.0) | 210,194 | 6.471 | 7.491 | not measured |
| 100 | True | rust | 3,576.1 (415.3–3,579.2) | 357,612 | 3.393 | 5.207 | 834.7 |
| 100 | True | go | 2,318.2 (65.2–2,343.9) | 231,816 | 6.483 | 8.935 | 618.0 |
| 100 | True | go-fasthttp | 2,397.8 (2,386.1–2,403.0) | 239,778 | 6.447 | 9.087 | 639.3 |
| 1,000 | False | rust | 599.4 (352.0–601.1) | 599,413 | 19.151 | 35.007 | not measured |
| 1,000 | False | go | 283.8 (277.6–284.4) | 283,847 | 63.231 | 127.231 | not measured |
| 1,000 | False | go-fasthttp | 279.9 (276.4–286.9) | 279,945 | 43.071 | 103.871 | not measured |
| 1,000 | True | rust | 379.0 (370.9–382.7) | 379,041 | 29.039 | 47.615 | 884.3 |
| 1,000 | True | go | 321.2 (317.4–326.3) | 321,226 | 41.471 | 77.183 | 854.6 |
| 1,000 | True | go-fasthttp | 328.4 (326.9–331.0) | 328,402 | 41.695 | 86.143 | 873.9 |
| 10,000 | False | rust | 64.6 (62.0–65.3) | 645,547 | 315.391 | 505.087 | not measured |
| 10,000 | False | go | 23.9 (23.8–24.2) | 238,932 | 442.879 | 632.319 | not measured |
| 10,000 | False | go-fasthttp | 25.6 (24.7–25.7) | 255,580 | 535.551 | 1025.023 | not measured |
| 10,000 | True | rust | 38.3 (37.8–39.2) | 383,095 | 401.407 | 459.775 | 892.9 |
| 10,000 | True | go | 26.9 (26.5–27.0) | 268,815 | 376.575 | 530.431 | 713.8 |
| 10,000 | True | go-fasthttp | 27.9 (27.5–28.6) | 278,544 | 542.207 | 851.967 | 739.6 |

#### Media and resources

| App | Upload + thumbnail median ms | Thumbnail bytes | Startup median ms | Idle Pss MiB | After HTTP Pss MiB | Final Pss MiB | Binary MiB |
|---|---:|---:|---:|---:|---:|---:|---:|
| rust | 29.95 | 84296 | 37.8 | 34.0 | 101.9 | 404.8 | 35.7 |
| go | 29.82 | 84296 | 54.4 | 31.4 | 130.2 | 1081.6 | 35.7 |
| go-fasthttp | 29.68 | 84296 | 55.0 | 32.9 | 141.1 | 2140.2 | 39.3 |

Startup includes application initialization, measured to a successful health request at 25 ms polling intervals. Final memory follows the complete workload sequence and includes allocator high-water effects; it is not a per-client memory measurement. Each media sample creates a new blob and fetches its generated thumbnail.

#### Limits

- Local, sequential, single-machine measurements; background activity was recorded (`metadata.json`), not
  eliminated, and the CPU governor was `powersave`. They are not a language-wide performance claim.
- The application listener is measured with identity encoding. TLS, ACME, gzip/zstd and the public response
  cache are not measured here.
- `go` is the unchanged `net/http` build, so the first three columns differ only in the HTTP layer; the Rust
  response bodies are larger and structured differently (see the response sizes in `report.md`), which the
  response contracts do not equate.
- All three applications used the same libvips 8.18.3 shared library, so media output is comparable but not
  the pinned container toolchain's byte goldens.

### Published cross-implementation results

Measured with 16 concurrent clients on an AMD Ryzen AI MAX+ 395 with 32 GB RAM,
with four hardware cores allocated to each app. The Go column was measured with the `net/http` build, before
the move to fasthttp, and has not been re-measured by that shared harness.

| HTTP workload (requests/sec) | Rails | [Django](https://github.com/basecamp/once-campfire-django) | [Laravel](https://github.com/basecamp/once-campfire-laravel) | [Express](https://github.com/basecamp/once-campfire-express) | [Elixir](https://github.com/basecamp/once-campfire-elixir) | [Go](https://github.com/basecamp/once-campfire-go) | [Rust](https://github.com/basecamp/once-campfire-rust) | [C](https://github.com/basecamp/once-campfire-c) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Room page | 4,132 | 478 | 3,872 | 42,636 | 5,350 | 53,060 | 106,494 | 137,524 |
| Messages page | 4,035 | 486 | 3,995 | 74,362 | 5,712 | 54,800 | 102,697 | 144,642 |
| Sidebar | 4,257 | 601 | 4,493 | 94,329 | 5,949 | 59,144 | 120,294 | 152,002 |
| Search | 4,216 | 594 | 4,172 | 84,665 | 5,848 | 60,509 | 121,378 | 149,487 |
| Post a message | 325 | 112 | 794 | 2,155 | 1,278 | 9,021 | 8,037 | 7,530 |

[Shared verification](https://github.com/basecamp/once-campfire-verification) · [Detailed results](https://github.com/basecamp/once-campfire-verification/blob/main/docs/performance-review.md).

See [`bench/`](bench/) for benchmark tooling and earlier measurements.

## Known differences

- **HTTP serving uses fasthttp (HTTP/1.1 only).** Rust and the previous Go build served HTTP/2 and,
  optionally, H2C; fasthttp has no HTTP/2, so TLS now negotiates `http/1.1` (plus the ACME challenge
  protocol) and `H2C_ENABLED` is accepted but ignored. Request bodies larger than the 64 KiB read buffer
  stream to the handler instead of being buffered (a 16 MiB body is tested; multipart parts still go to disk),
  and stored files above 256 KiB stream from the file descriptor after the handler returns.
  `ReadHeaderTimeout` is gone (fasthttp has a single read timeout) and request headers are limited to
  64 KiB. ACME HTTP-01 challenges and the plain-HTTP redirect are answered by
  autocert's own handler through `fasthttpadaptor`. Live Pebble ACME (`bin/check-acme`) was not re-run
  against this change; the TLS handshake, certificate cache and redirect are covered by `internal/front` tests.
- Outbound HTTP (webhooks, Open Graph link previews, Web Push) still uses `net/http`, which provides the
  guarded dialer, redirect policy and Net::HTTP-style inflation those features need.
- Action Cable upgrades hijack the fasthttp connection; the WebSocket handshake reuses coder/websocket's
  verification and negotiation through a small addition to the local fork
  (`third_party/websocket/handshake.go`).

Browser writes use Rust’s token-free Fetch Metadata policy: exact `same-origin` or
`same-site`, a matching Origin when provided, and absent metadata only on plain
HTTP with TLS disabled. Signed disk uploads retain their authenticated capability
exemption; bot-key endpoints exempt authenticated bots only.


- Session-transfer auto-submit forms explicitly close their form tag; the pinned Rails
  reference omitted it.
- Background sidebar refreshes preserve an open New Ping form and selected recipients.

- Sidebar connection refresh waits for the current Turbo frame to finish loading,
  preventing an aborted response on startup or reconnect. Obsolete connections and removed frames do not reload.

- Search selects the newest 100 matching messages by insertion ID, then displays them in ID order. Backdated messages can appear in a different order from the original Rails app.

- Templates use `html/template`. Whitespace, attribute serialization, some canonical form-action
  URLs, and response headers/validators differ from Rust. Strict server/live DOM and network layers
  therefore still fail in many inventory cells, even when screenshots, accessibility and workflows
  match. These failures remain visible in the validation report. Exact protocol parity for malformed
  parameters and every content-negotiation edge case is not claimed.
- WebSockets share serialized and compressed broadcast payloads through a small extension to
  coder/websocket v1.8.15 (see `third_party/websocket/README.campfire`). Outgoing queues hold 256
  frames; slow clients are disconnected. Authorization is checked afresh for each publication,
  batching distinct sessions per room. Rust uses different stream queues.
- Go ignores typing commands for rooms that have been deleted; Rust can still echo them to an
  already subscribed socket. The composer shows the same deleted-room message.
- The response cache uses least-recently-used eviction instead of Rust's sampled eviction. The Go
  message-fragment cache is also independently implemented. It retains versioned message lists
  and sidebar HTML; current membership and permission data are read before cache lookup.
  Room pages also cache their surrounding HTML keyed by fresh page data, inserting freshly selected
  messages and their queried refresh cursor. Search layouts similarly surround a captured immutable
  result body; misses rerun the scoped full query. Complete GET gzip representations have a bounded
  memo keyed by immutable part identities, not by wire ETags. Authorization, sessions and request
  observations stay fresh. Responses derive validators from part lengths and hashes, so ETag values
  differ from both the original Go implementation and Rust. Message pagination uses its
  complete rendered body for ETags, including related-user and boost changes. It omits
  Last-Modified because message timestamps cannot describe external presentation edits.
- Completed room, messages, sidebar and search HTML/gzip responses share a per-process
  cache (`CAMPFIRE_RESPONSE_CACHE_MB`, default 64 MiB, 0 disables it). Every request checks
  its session and access before lookup. A dedicated SQLite reader observes local and foreign
  commits and invalidates whole responses. Message and message-list fragments also carry
  the observed generation and request origin, including external edits without timestamp
  changes and host-dependent rich-text filtering. Request
  variants remain separate; headers and cookies
  stay fresh. GET and HEAD share bodies and preserve validators; flash responses bypass it.
- The default version label and fallback VAPID subject identify `once-campfire-go`. Explicit version,
  VAPID keys and subject settings remain supported.
- Storage keys containing separators, NUL, or parent-directory shards are rejected before
  filesystem access. Valid keys retain the existing storage layout.
- Native host media output can differ with installed library versions. All byte-golden media tests
  pass with the pinned container libraries. Web Push is verified locally, not against external push
  providers.

As in Rust, background queues are bounded and in-process: graceful shutdown drains work, but a
process crash can lose queued jobs. See [the implementation record](plans/go-conversion.md) for
coverage and validation scope.

## License

MIT; see [MIT-LICENSE](MIT-LICENSE). The copyright is held by 37signals, LLC (Campfire) and
Manos Ragiadakos (the Go port and the fasthttp transport). [NOTICE](NOTICE) lists the upstream work
this port derives from and the bundled and adapted third-party code. Their license texts are in
[licenses](licenses/), [internal/html/LICENSE](internal/html/LICENSE), and
[third_party/websocket/LICENSE.txt](third_party/websocket/LICENSE.txt). Module dependencies retain
their own licenses.
