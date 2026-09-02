/* Use the ten-second timing floor for the strict-boundary driver. */
#define GTS_TIMING_ORACLE_MIN_ELAPSED_NS UINT64_C(10000000000)
#include "go_timing_oracle.c"
