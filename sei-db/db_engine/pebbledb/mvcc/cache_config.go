package mvcc

import (
	"fmt"
	"math"
	"os"
	"strconv"
)

const blockCacheMiBEnv = "SEI_SS_MVCC_BLOCK_CACHE_MIB"

func blockCacheBytes() (int64, error) {
	value, set := os.LookupEnv(blockCacheMiBEnv)
	if !set {
		return 32 << 20, nil
	}
	mib, err := strconv.ParseInt(value, 10, 64)
	if err != nil || mib <= 0 || mib > math.MaxInt64>>20 {
		return 0, fmt.Errorf("%s must be a positive integer MiB capacity no larger than %d", blockCacheMiBEnv, math.MaxInt64>>20)
	}
	return mib << 20, nil
}
