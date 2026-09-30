Each row reports medians from 20 paired randomized processes. Timing excludes fresh-tree setup and includes Tree.Edit. Standard B/op and allocs/op cover a four-edit cycle and omit C native allocations; Go and C columns cover one edit. PowerShell* includes the existing dependency fix in both measurement checkouts.

| Language | Target / actual bytes | Edit | Go ms before → after | C ms before → after | Go/C before → after | Go time change | Cycle B/op before → after | Cycle allocs/op before → after |
|---|---|---|---:|---:|---:|---:|---:|---:|
| go | 32768 / 32686 | 100_byte | 123.202 → 124.697 | 0.327 → 0.329 | 372.50 → 375.25 | +1.21% | 28,331,702 → 28,331,702 | 533.0 → 535.5 |
| go | 32768 / 32686 | one_byte | 122.195 → 124.254 | 0.326 → 0.327 | 376.10 → 378.80 | +1.69% | 28,225,188 → 28,225,158 | 532.0 → 534.0 |
| go | 32768 / 32686 | splice | 127.729 → 127.461 | 2.615 → 2.805 | 48.58 → 43.63 | -0.21% | 32,481,776 → 32,481,776 | 613.0 → 613.0 |
| go | 140288 / 140026 | 100_byte | 243.417 → 244.626 | 0.637 → 0.640 | 387.85 → 385.65 | +0.50% | 32,338,116 → 32,338,116 | 320.0 → 320.0 |
| go | 140288 / 140026 | one_byte | 243.712 → 243.779 | 0.624 → 0.634 | 387.65 → 387.80 | +0.03% | 32,337,962 → 32,337,962 | 319.0 → 319.0 |
| go | 140288 / 140026 | splice | 308.089 → 314.023 | 102.394 → 102.700 | 2.98 → 2.94 | +1.93% | 83,616,244 → 83,616,220 | 17,486.5 → 17,441.5 |
| javascript | 32768 / 32689 | 100_byte | 26.863 → 28.062 | 0.697 → 0.698 | 38.84 → 40.00 | +4.46% | 2,302,268 → 2,302,294 | 24,183.0 → 24,183.0 |
| javascript | 32768 / 32689 | one_byte | 26.471 → 26.511 | 0.698 → 0.692 | 38.20 → 38.61 | +0.15% | 1,450,242 → 1,450,260 | 24,183.0 → 24,183.0 |
| javascript | 32768 / 32689 | splice | 22.195 → 22.195 | 0.980 → 1.008 | 21.92 → 22.11 | -0.00% | 4,646,912 → 4,647,464 | 911.0 → 911.0 |
| javascript | 140288 / 141043 | 100_byte | 8.185 → 8.365 | 0.391 → 0.389 | 19.95 → 20.39 | +2.20% | 7,124,810 → 7,124,807 | 73.0 → 73.0 |
| javascript | 140288 / 141043 | one_byte | 7.740 → 8.178 | 0.381 → 0.383 | 20.47 → 20.55 | +5.66% | 7,124,803 → 7,124,806 | 73.0 → 73.0 |
| javascript | 140288 / 141043 | splice | 7.795 → 8.175 | 0.372 → 0.389 | 20.44 → 20.51 | +4.88% | 7,124,802 → 7,124,810 | 73.0 → 73.0 |
| typescript | 32768 / 32652 | 100_byte | 22.680 → 23.076 | 3.293 → 3.316 | 6.87 → 7.03 | +1.74% | 4,061,196 → 4,061,026 | 504.5 → 505.0 |
| typescript | 32768 / 32652 | one_byte | 22.626 → 22.944 | 3.285 → 3.252 | 6.89 → 7.06 | +1.41% | 4,062,938 → 4,062,940 | 505.0 → 504.0 |
| typescript | 32768 / 32652 | splice | 107.758 → 108.392 | 7.676 → 7.725 | 14.05 → 13.97 | +0.59% | 13,477,804 → 13,477,810 | 19,334.0 → 19,334.0 |
| typescript | 140288 / 140212 | 100_byte | 378.556 → 375.283 | 90.328 → 89.916 | 4.12 → 4.15 | -0.86% | 44,923,520 → 44,923,520 | 390.0 → 390.0 |
| typescript | 140288 / 140212 | one_byte | 367.193 → 367.638 | 88.534 → 89.920 | 4.10 → 4.10 | +0.12% | 20,941,640 → 20,941,640 | 359.5 → 363.5 |
| typescript | 140288 / 140212 | splice | 421.841 → 420.529 | 90.901 → 91.276 | 4.66 → 4.58 | -0.31% | 84,470,560 → 84,470,560 | 1,083.0 → 1,083.5 |
| python | 32768 / 32768 | 100_byte | 86.360 → 86.360 | 5.767 → 5.801 | 14.97 → 14.87 | -0.00% | 21,586,242 → 21,586,240 | 2,590.0 → 2,590.0 |
| python | 32768 / 32768 | one_byte | 85.614 → 85.069 | 5.748 → 5.746 | 14.81 → 14.70 | -0.64% | 21,590,067 → 21,590,066 | 2,613.0 → 2,613.0 |
| python | 32768 / 32768 | splice | 38.526 → 38.342 | 2.111 → 2.121 | 18.18 → 18.14 | -0.48% | 3,597,046 → 3,597,046 | 521.0 → 521.0 |
| python | 140288 / 139106 | 100_byte | 164.664 → 164.012 | 6.488 → 6.496 | 25.44 → 25.34 | -0.40% | 11,328,368 → 11,328,368 | 621.0 → 621.0 |
| python | 140288 / 139106 | one_byte | 164.944 → 165.724 | 6.525 → 6.569 | 25.40 → 25.55 | +0.47% | 11,328,368 → 11,328,368 | 621.0 → 621.0 |
| python | 140288 / 139106 | splice | 162.377 → 162.604 | 11.693 → 11.789 | 13.97 → 13.81 | +0.14% | 10,925,008 → 10,925,008 | 599.0 → 599.0 |
| rust | 32768 / 32765 | 100_byte | 39.408 → 39.181 | 0.424 → 0.420 | 93.08 → 93.02 | -0.58% | 9,198,668 → 9,198,668 | 570.0 → 570.0 |
| rust | 32768 / 32765 | one_byte | 39.564 → 39.639 | 0.417 → 0.414 | 95.09 → 94.84 | +0.19% | 9,206,738 → 9,206,734 | 572.0 → 573.0 |
| rust | 32768 / 32765 | splice | 38.522 → 38.433 | 0.432 → 0.433 | 89.21 → 88.45 | -0.23% | 9,207,551 → 9,207,555 | 622.0 → 622.0 |
| rust | 140288 / 140019 | 100_byte | 735.394 → 735.215 | 1.525 → 1.528 | 484.60 → 479.40 | -0.02% | 109,315,384 → 109,315,408 | 1,190.0 → 1,190.0 |
| rust | 140288 / 140019 | one_byte | 734.648 → 730.348 | 1.501 → 1.517 | 489.90 → 480.45 | -0.59% | 97,471,956 → 97,399,124 | 1,107.5 → 1,106.0 |
| rust | 140288 / 140019 | splice | 548.724 → 544.856 | 0.992 → 0.994 | 549.20 → 549.05 | -0.70% | 66,097,064 → 66,097,088 | 1,347.0 → 1,347.0 |
| java | 32768 / 32728 | 100_byte | 12.377 → 12.323 | 0.364 → 0.364 | 33.95 → 33.93 | -0.44% | 6,885,540 → 6,885,541 | 230.0 → 230.0 |
| java | 32768 / 32728 | one_byte | 12.305 → 12.170 | 0.365 → 0.365 | 32.90 → 33.68 | -1.10% | 6,902,644 → 6,902,643 | 230.0 → 230.0 |
| java | 32768 / 32728 | splice | 11.889 → 12.001 | 0.361 → 0.362 | 33.08 → 33.17 | +0.94% | 6,894,320 → 6,894,499 | 232.0 → 232.0 |
| java | 140288 / 144870 | 100_byte | 197.580 → 199.111 | 1.511 → 1.469 | 132.90 → 135.75 | +0.77% | 114,942,580 → 115,072,052 | 4,141.0 → 4,140.5 |
| java | 140288 / 144870 | one_byte | 197.171 → 197.885 | 1.455 → 1.456 | 135.80 → 136.95 | +0.36% | 114,940,980 → 114,940,980 | 4,140.5 → 4,140.0 |
| java | 140288 / 144870 | splice | 1177.841 → 1157.943 | 1.640 → 1.605 | 746.15 → 739.05 | -1.69% | 236,819,816 → 236,819,816 | 30,115.0 → 30,115.0 |
| c_sharp | 32768 / 32846 | 100_byte | 471.773 → 476.073 | 0.791 → 0.794 | 592.55 → 598.95 | +0.91% | 164,377,080 → 164,373,540 | 85,365.5 → 85,370.0 |
| c_sharp | 32768 / 32846 | one_byte | 465.911 → 466.726 | 0.789 → 0.790 | 588.70 → 586.35 | +0.17% | 161,211,488 → 162,824,592 | 85,355.5 → 85,340.0 |
| c_sharp | 32768 / 32846 | splice | 394.526 → 391.146 | 0.571 → 0.574 | 679.05 → 681.45 | -0.86% | 115,252,598 → 115,255,164 | 56,461.0 → 56,424.5 |
| c_sharp | 140288 / 134787 | 100_byte | 5074.947 → 5079.783 | 0.778 → 0.780 | 6571.50 → 6520.00 | +0.10% | 87,360,556 → 87,359,208 | 144,816.0 → 144,823.5 |
| c_sharp | 140288 / 134787 | one_byte | 4909.871 → 4924.173 | 0.781 → 0.779 | 6438.50 → 6321.50 | +0.29% | 87,063,636 → 87,065,800 | 140,835.0 → 140,844.0 |
| c_sharp | 140288 / 134787 | splice | 2171.973 → 2174.715 | 5.091 → 5.208 | 425.95 → 415.60 | +0.13% | 165,703,320 → 165,707,936 | 111,346.0 → 111,344.0 |
| powershell* | 32768 / 32666 | 100_byte | 32.637 → 32.982 | 0.136 → 0.138 | 238.50 → 239.50 | +1.06% | 3,761,441 → 3,761,445 | 1,003.0 → 1,003.0 |
| powershell* | 32768 / 32666 | one_byte | 32.496 → 32.560 | 0.135 → 0.136 | 240.65 → 239.65 | +0.20% | 3,761,443 → 3,761,441 | 1,003.0 → 1,003.0 |
| powershell* | 32768 / 32666 | splice | 32.989 → 32.746 | 0.136 → 0.136 | 242.00 → 239.90 | -0.74% | 3,761,441 → 3,761,441 | 1,003.0 → 1,003.0 |
| powershell* | 140288 / 104415 | 100_byte | 2284.132 → 2287.905 | 0.321 → 0.320 | 7174.50 → 7067.50 | +0.17% | 105,704,568 → 105,723,008 | 21,486.0 → 21,481.0 |
| powershell* | 140288 / 104415 | one_byte | 2285.893 → 2280.704 | 0.296 → 0.303 | 7735.00 → 7525.00 | -0.23% | 105,667,616 → 105,704,520 | 21,479.5 → 21,481.0 |
| powershell* | 140288 / 104415 | splice | 2271.801 → 2285.543 | 0.326 → 0.319 | 6934.50 → 7085.00 | +0.60% | 105,691,552 → 105,746,896 | 21,875.5 → 21,872.0 |

Directional timing changes are reported for every workload, including nonsignificant increases. See the per-language benchstat files for confidence intervals and p-values; observer-enabled census timings are separate. Go/C ratios on unequal parse results are diagnostic costs.

```text
METRIC: go/32768/100_byte edit Go/C | 372.500 -> 375.250 | 6f17ca1d6..592c8169d (default-off hooks) | 32686 bytes, middle-site 100_byte, 20 seeds
METRIC: go/32768/one_byte edit Go/C | 376.100 -> 378.800 | 6f17ca1d6..592c8169d (default-off hooks) | 32686 bytes, middle-site one_byte, 20 seeds
METRIC: go/32768/splice edit Go/C | 48.575 -> 43.635 | 6f17ca1d6..592c8169d (default-off hooks) | 32686 bytes, middle-site splice, 20 seeds
METRIC: go/140288/100_byte edit Go/C | 387.850 -> 385.650 | 6f17ca1d6..592c8169d (default-off hooks) | 140026 bytes, middle-site 100_byte, 20 seeds
METRIC: go/140288/one_byte edit Go/C | 387.650 -> 387.800 | 6f17ca1d6..592c8169d (default-off hooks) | 140026 bytes, middle-site one_byte, 20 seeds
METRIC: go/140288/splice edit Go/C | 2.983 -> 2.942 | 6f17ca1d6..592c8169d (default-off hooks) | 140026 bytes, middle-site splice, 20 seeds
METRIC: javascript/32768/100_byte edit Go/C | 38.845 -> 40.005 | 592c8169d..fea550641 (default-off hooks) | 32689 bytes, middle-site 100_byte, 20 seeds
METRIC: javascript/32768/one_byte edit Go/C | 38.205 -> 38.605 | 592c8169d..fea550641 (default-off hooks) | 32689 bytes, middle-site one_byte, 20 seeds
METRIC: javascript/32768/splice edit Go/C | 21.920 -> 22.105 | 592c8169d..fea550641 (default-off hooks) | 32689 bytes, middle-site splice, 20 seeds
METRIC: javascript/140288/100_byte edit Go/C | 19.950 -> 20.390 | 592c8169d..fea550641 (default-off hooks) | 141043 bytes, middle-site 100_byte, 20 seeds
METRIC: javascript/140288/one_byte edit Go/C | 20.470 -> 20.555 | 592c8169d..fea550641 (default-off hooks) | 141043 bytes, middle-site one_byte, 20 seeds
METRIC: javascript/140288/splice edit Go/C | 20.440 -> 20.505 | 592c8169d..fea550641 (default-off hooks) | 141043 bytes, middle-site splice, 20 seeds
METRIC: typescript/32768/100_byte edit Go/C | 6.865 -> 7.027 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32652 bytes, middle-site 100_byte, 20 seeds
METRIC: typescript/32768/one_byte edit Go/C | 6.889 -> 7.056 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32652 bytes, middle-site one_byte, 20 seeds
METRIC: typescript/32768/splice edit Go/C | 14.055 -> 13.970 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32652 bytes, middle-site splice, 20 seeds
METRIC: typescript/140288/100_byte edit Go/C | 4.118 -> 4.147 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140212 bytes, middle-site 100_byte, 20 seeds
METRIC: typescript/140288/one_byte edit Go/C | 4.104 -> 4.095 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140212 bytes, middle-site one_byte, 20 seeds
METRIC: typescript/140288/splice edit Go/C | 4.663 -> 4.581 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140212 bytes, middle-site splice, 20 seeds
METRIC: python/32768/100_byte edit Go/C | 14.970 -> 14.870 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32768 bytes, middle-site 100_byte, 20 seeds
METRIC: python/32768/one_byte edit Go/C | 14.810 -> 14.700 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32768 bytes, middle-site one_byte, 20 seeds
METRIC: python/32768/splice edit Go/C | 18.180 -> 18.140 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32768 bytes, middle-site splice, 20 seeds
METRIC: python/140288/100_byte edit Go/C | 25.440 -> 25.345 | fea550641ccc92cbc157c6c5545d77a44818a745 | 139106 bytes, middle-site 100_byte, 20 seeds
METRIC: python/140288/one_byte edit Go/C | 25.400 -> 25.545 | fea550641ccc92cbc157c6c5545d77a44818a745 | 139106 bytes, middle-site one_byte, 20 seeds
METRIC: python/140288/splice edit Go/C | 13.970 -> 13.810 | fea550641ccc92cbc157c6c5545d77a44818a745 | 139106 bytes, middle-site splice, 20 seeds
METRIC: rust/32768/100_byte edit Go/C | 93.080 -> 93.015 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32765 bytes, middle-site 100_byte, 20 seeds
METRIC: rust/32768/one_byte edit Go/C | 95.095 -> 94.845 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32765 bytes, middle-site one_byte, 20 seeds
METRIC: rust/32768/splice edit Go/C | 89.205 -> 88.455 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32765 bytes, middle-site splice, 20 seeds
METRIC: rust/140288/100_byte edit Go/C | 484.600 -> 479.400 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140019 bytes, middle-site 100_byte, 20 seeds
METRIC: rust/140288/one_byte edit Go/C | 489.900 -> 480.450 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140019 bytes, middle-site one_byte, 20 seeds
METRIC: rust/140288/splice edit Go/C | 549.200 -> 549.050 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140019 bytes, middle-site splice, 20 seeds
METRIC: java/32768/100_byte edit Go/C | 33.955 -> 33.930 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32728 bytes, middle-site 100_byte, 20 seeds
METRIC: java/32768/one_byte edit Go/C | 32.900 -> 33.680 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32728 bytes, middle-site one_byte, 20 seeds
METRIC: java/32768/splice edit Go/C | 33.075 -> 33.175 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32728 bytes, middle-site splice, 20 seeds
METRIC: java/140288/100_byte edit Go/C | 132.900 -> 135.750 | fea550641ccc92cbc157c6c5545d77a44818a745 | 144870 bytes, middle-site 100_byte, 20 seeds
METRIC: java/140288/one_byte edit Go/C | 135.800 -> 136.950 | fea550641ccc92cbc157c6c5545d77a44818a745 | 144870 bytes, middle-site one_byte, 20 seeds
METRIC: java/140288/splice edit Go/C | 746.150 -> 739.050 | fea550641ccc92cbc157c6c5545d77a44818a745 | 144870 bytes, middle-site splice, 20 seeds
METRIC: c_sharp/32768/100_byte edit Go/C | 592.550 -> 598.950 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32846 bytes, middle-site 100_byte, 20 seeds
METRIC: c_sharp/32768/one_byte edit Go/C | 588.700 -> 586.350 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32846 bytes, middle-site one_byte, 20 seeds
METRIC: c_sharp/32768/splice edit Go/C | 679.050 -> 681.450 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32846 bytes, middle-site splice, 20 seeds
METRIC: c_sharp/140288/100_byte edit Go/C | 6571.500 -> 6520.000 | fea550641ccc92cbc157c6c5545d77a44818a745 | 134787 bytes, middle-site 100_byte, 20 seeds
METRIC: c_sharp/140288/one_byte edit Go/C | 6438.500 -> 6321.500 | fea550641ccc92cbc157c6c5545d77a44818a745 | 134787 bytes, middle-site one_byte, 20 seeds
METRIC: c_sharp/140288/splice edit Go/C | 425.950 -> 415.600 | fea550641ccc92cbc157c6c5545d77a44818a745 | 134787 bytes, middle-site splice, 20 seeds
METRIC: powershell/32768/100_byte edit Go/C | 238.500 -> 239.500 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 32666 bytes, middle-site 100_byte, 20 seeds
METRIC: powershell/32768/one_byte edit Go/C | 240.650 -> 239.650 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 32666 bytes, middle-site one_byte, 20 seeds
METRIC: powershell/32768/splice edit Go/C | 242.000 -> 239.900 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 32666 bytes, middle-site splice, 20 seeds
METRIC: powershell/140288/100_byte edit Go/C | 7174.500 -> 7067.500 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 104415 bytes, middle-site 100_byte, 20 seeds
METRIC: powershell/140288/one_byte edit Go/C | 7735.000 -> 7525.000 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 104415 bytes, middle-site one_byte, 20 seeds
METRIC: powershell/140288/splice edit Go/C | 6934.500 -> 7085.000 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 104415 bytes, middle-site splice, 20 seeds
```

Source limits: the early Go and JavaScript runs span default-off hook refinements, with unchanged policy and independently checked results/counters. Their METRIC revision fields show ranges. The final primary trio uses the frozen instrument. Optional real-source repeats were not started because the remaining time was insufficient. Container Git metadata is unavailable; the host worktree revisions and raw hashes provide provenance, rather than an authenticated container source snapshot.

Final primary Go trio (20 paired seeds, 750 ms, GOMAXPROCS=1, count=1, benchmem):

```text
goos: linux
goarch: amd64
pkg: github.com/odvcencio/gotreesitter
cpu: Intel(R) Xeon(R) Platinum 8481C CPU @ 2.70GHz
                                    │ trio-before.txt │           trio-after.txt           │
                                    │     sec/op      │   sec/op     vs base               │
GoParseFullDFA                            8.873m ± 0%   8.871m ± 0%       ~ (p=0.989 n=20)
GoParseIncrementalSingleByteEditDFA       196.2µ ± 0%   198.2µ ± 1%  +1.03% (p=0.000 n=20)
GoParseIncrementalNoEditDFA               8.512n ± 3%   8.444n ± 1%       ~ (p=0.057 n=20)
geomean                                   24.56µ        24.58µ       +0.06%

                                    │ trio-before.txt │           trio-after.txt            │
                                    │       B/s       │     B/s       vs base               │
GoParseFullDFA                           2.069Mi ± 0%   2.074Mi ± 0%       ~ (p=0.753 n=20)
GoParseIncrementalSingleByteEditDFA      93.78Mi ± 0%   92.82Mi ± 1%  -1.02% (p=0.000 n=20)
GoParseIncrementalNoEditDFA              2.061Ti ± 3%   2.078Ti ± 1%       ~ (p=0.060 n=20)
geomean                                  748.6Mi        748.6Mi       +0.00%

                                    │ trio-before.txt │            trio-after.txt             │
                                    │      B/op       │     B/op      vs base                 │
GoParseFullDFA                         1.246Ki ± 0%     1.246Ki ± 0%       ~ (p=0.854 n=20)
GoParseIncrementalSingleByteEditDFA      388.0 ± 0%       388.0 ± 0%       ~ (p=1.000 n=20)
GoParseIncrementalNoEditDFA              0.000 ± 0%       0.000 ± 0%       ~ (p=1.000 n=20) ¹
geomean                                             ²                 +0.00%                ²
¹ all samples are equal
² summaries must be >0 to compute geomean

                                    │ trio-before.txt │           trio-after.txt            │
                                    │    allocs/op    │ allocs/op   vs base                 │
GoParseFullDFA                           8.000 ± 0%     8.000 ± 0%       ~ (p=1.000 n=20) ¹
GoParseIncrementalSingleByteEditDFA      5.000 ± 0%     5.000 ± 0%       ~ (p=1.000 n=20) ¹
GoParseIncrementalNoEditDFA              0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=20) ¹
geomean                                             ²               +0.00%                ²
¹ all samples are equal
² summaries must be >0 to compute geomean
```

All directional changes remain in the workload table and benchstat files. None of the 48 real-source Go edit-time comparisons had a significant increase. The frozen primary control edit increased **1.03%** (196.2 → 198.2 µs, p<0.001); B/op remains 388 and allocs/op remains 5. Full parsing and no-edit time show no significant change; no-edit remains 0 B/op and 0 allocs/op. This small control timing regression remains unexplained; shared-host variation and binary layout have not been isolated. It is recorded rather than claimed as zero cost. The Go 32 KiB splice C timer rose 7.26% (p=0.028), with an after interval of ±55%; that variation occurred in the C control. No optimization speedup is claimed. Diagnostic peak RSS remains in REPORT.md; benchmark processes had no crash, OOM, or wall timeout.
