# Campfire application benchmark

Release binaries; application HTTP/1.1 listener; identity encoding; identical seed and CPU affinity. Media use installed native libraries. Host background load recorded.

3 rotating repetitions; 5.0-second HTTP samples after 2-second warmups. Four application workers on CPUs 8-11; load generator on CPUs 12-15. HTTP uses identity encoding. Every run starts with a separate copy of the same seed.

HTTP response contracts compare message IDs, sidebar room IDs, avatar bytes and CSS bytes. Opt-in message forms require complete document layouts and seed-grounded editor/attachment/ordered boost controls; selected applications also require exact decoded form bytes after normalization of only their listener origin. These are full-document GETs, not Turbo-Frame click measurements. Write checks require every successful request to persist and enter the FTS index. Cable checks require every client to subscribe and every message to arrive at every client. Upload checks fetch the actual representation and compare its bytes across applications.

## HTTP

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

## Action Cable

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

## Media and resources

| App | Upload + thumbnail median ms | Thumbnail bytes | Startup median ms | Idle Pss MiB | After HTTP Pss MiB | Final Pss MiB | Binary MiB |
|---|---:|---:|---:|---:|---:|---:|---:|
| rust | 29.95 | 84296 | 37.8 | 34.0 | 101.9 | 404.8 | 35.7 |
| go | 29.82 | 84296 | 54.4 | 31.4 | 130.2 | 1081.6 | 35.7 |
| go-fasthttp | 29.68 | 84296 | 55.0 | 32.9 | 141.1 | 2140.2 | 39.3 |

Startup includes application initialization, measured to a successful health request at 25 ms polling intervals. Final memory follows the complete workload sequence and includes allocator high-water effects; it is not a per-client memory measurement. Each media sample creates a new blob and fetches its generated thumbnail.

## Reproduction and limits

- [Raw samples](raw.json) include statuses, errors, latency distributions, delivery counters, byte hashes and memory snapshots. [Metadata](metadata.json) records source/binary/seed hashes, CPU details, affinity and load averages.
- These are local workstation measurements, sequential within the harness. Background host activity is recorded, not eliminated. They are not a language-wide performance claim.
- This comparison uses the application HTTP listener and identity encoding. It does not measure TLS/ACME or zstd throughput. Active-room CPU includes the concurrent mutation writer.
- Sidebar room-ID checks do not validate application/Turbo-frame layout completeness; verify that scope separately before any cross-port sidebar comparison.
- Screen-level HTML and network comparisons remain stricter than the functional response contracts used here. Benchmark validation is not a declaration of complete byte-for-byte UI parity.

## Validation totals

9 completed application runs; 694,153 acknowledged HTTP posts and 0 active-room posts verified in messages and FTS; 45 validated thumbnail samples. HTTP errors: 0. Incomplete throughput deliveries: 0.

Initial full response sizes from the first repetition, before workload timing. Message/room ID and byte contracts are recorded in raw.json. Benchmark contracts do not establish browser or strict HTML parity.

| Response | App | Plain bytes | Encoded bytes | Encoding |
|---|---|---:|---:|---|
| room_show | rust | 416,139 | 416139 | identity |
| messages_page | rust | 383,844 | 383844 | identity |
| sidebar | rust | 30,763 | 30763 | identity |
| search | rust | 149,625 | 149625 | identity |
| avatar | rust | 3,364 | 3364 | identity |
| static_css | rust | 1,218 | 1218 | identity |
| room_show | go | 374,036 | 374036 | identity |
| messages_page | go | 342,444 | 342444 | identity |
| sidebar | go | 29,684 | 29684 | identity |
| search | go | 135,497 | 135497 | identity |
| avatar | go | 3,364 | 3364 | identity |
| static_css | go | 1,218 | 1218 | identity |
| room_show | go-fasthttp | 374,036 | 374036 | identity |
| messages_page | go-fasthttp | 342,444 | 342444 | identity |
| sidebar | go-fasthttp | 29,684 | 29684 | identity |
| search | go-fasthttp | 135,497 | 135497 | identity |
| avatar | go-fasthttp | 3,364 | 3364 | identity |
| static_css | go-fasthttp | 1,218 | 1218 | identity |

Application logging is retained. Settings, native libraries, source/binary hashes and toolchains are recorded in metadata.json. Container media byte goldens require their separately pinned toolchain.
