W5 editor latency: **passed**

Base `f9828512cc58f1bc77b856c2b3451e85266c38b1`; head `dcbe01c6e5fec4940e3c243c3a51af0390ef7656`. Twenty paired shuffle seeds, 750ms, one thread. A paired median above +5% fails any cell.

| Language | Edit | Go base → head (µs/edit) | C base → head (µs/edit) | Go/C base → head | Paired Go change |
|---|---|---:|---:|---:|---:|
| typescript | one_byte | 15572.08 → 15582.91 | 89.40 → 87.96 | 177.06x → 176.17x | +0.79% PASS |
| typescript | hundred_byte | 14814.80 → 13548.06 | 75.53 → 57.70 | 233.73x → 234.54x | -0.37% PASS |
| typescript | typing | 12104.09 → 11059.56 | 48.31 → 45.43 | 246.51x → 248.14x | -0.31% PASS |
| tsx | one_byte | 3266.42 → 3260.02 | 13.64 → 13.71 | 242.84x → 240.62x | -0.03% PASS |
| tsx | hundred_byte | 4188.29 → 4174.49 | 62.60 → 62.39 | 66.95x → 66.90x | -0.14% PASS |
| tsx | typing | 1446.15 → 1447.59 | 14.96 → 14.97 | 96.73x → 96.87x | +0.20% PASS |
| javascript | one_byte | 10928.05 → 10737.09 | 765.48 → 755.40 | 14.28x → 14.25x | -0.75% PASS |
| javascript | hundred_byte | 11000.64 → 10962.45 | 768.64 → 763.79 | 14.29x → 14.41x | -0.36% PASS |
| javascript | typing | 10145.38 → 10269.09 | 885.47 → 878.34 | 11.53x → 11.71x | -0.75% PASS |
| python | one_byte | 6167.06 → 6161.90 | 38.96 → 38.60 | 160.11x → 160.35x | -0.07% PASS |
| python | hundred_byte | 7379.11 → 7376.16 | 49.66 → 49.83 | 147.94x → 147.95x | +0.08% PASS |
| python | typing | 7142.51 → 7077.09 | 79.30 → 79.43 | 89.36x → 88.61x | -0.88% PASS |
| java | one_byte | 13904.36 → 13963.35 | 35.03 → 35.17 | 397.93x → 397.34x | +0.22% PASS |
| java | hundred_byte | 14089.26 → 13981.40 | 165.44 → 164.40 | 85.46x → 84.60x | -0.12% PASS |
| java | typing | 15051.77 → 15144.22 | 29.28 → 29.27 | 512.75x → 516.26x | +0.40% PASS |
| c_sharp | one_byte | 521241.34 → 521434.76 | 34.68 → 35.09 | 15368.04x → 15473.15x | +0.09% PASS |
| c_sharp | hundred_byte | 516485.39 → 520152.44 | 326.29 → 330.64 | 1629.93x → 1634.78x | +0.33% PASS |
| c_sharp | typing | 520995.15 → 519918.31 | 22.09 → 23.29 | 23648.15x → 23509.63x | -0.26% PASS |
| php | one_byte | 17224.76 → 16978.62 | 30.39 → 30.43 | 560.73x → 556.98x | +0.24% PASS |
| php | hundred_byte | 17075.92 → 16986.22 | 179.18 → 175.85 | 95.72x → 96.98x | -0.13% PASS |
| php | typing | 16914.82 → 16834.46 | 24.55 → 24.37 | 700.04x → 695.99x | -0.10% PASS |
| bash | one_byte | 1592.63 → 1592.10 | 332.62 → 333.14 | 4.79x → 4.78x | -0.02% PASS |
| bash | hundred_byte | 1598.67 → 1599.43 | 312.89 → 312.27 | 5.11x → 5.12x | +0.04% PASS |
| bash | typing | 1595.31 → 1594.89 | 321.44 → 319.74 | 4.98x → 4.99x | -0.02% PASS |
| go | one_byte | 87944.50 → 87911.82 | 285.57 → 284.81 | 309.81x → 308.68x | -0.14% PASS |
| go | hundred_byte | 89691.60 → 88804.13 | 352.70 → 353.12 | 252.91x → 250.94x | -0.41% PASS |
| go | typing | 85085.34 → 84899.42 | 211.24 → 210.97 | 402.08x → 402.10x | -0.21% PASS |
| powershell | one_byte | 5248.33 → 5249.31 | 76.23 → 76.28 | 68.69x → 68.74x | +0.09% PASS |
| powershell | hundred_byte | 5297.92 → 5318.43 | 85.17 → 85.11 | 62.42x → 62.45x | +0.24% PASS |
| powershell | typing | 1871.37 → 1862.56 | 18.50 → 18.53 | 100.71x → 100.00x | -0.50% PASS |

C timing and the range of paired Go samples are recorded in receipt.json. C drift is reported; it does not excuse a Go regression. Tree.Edit, reparse, and previous-tree release are timed. Go fresh resets and C snapshot restoration are excluded from edit timing. C typing allocations include the small snapshot clone; C allocations cover the Go binding only; Go allocations include parser work.
