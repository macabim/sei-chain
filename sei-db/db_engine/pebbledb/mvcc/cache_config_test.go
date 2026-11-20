package mvcc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sei-protocol/sei-chain/sei-db/config"
)

func TestBlockCacheBytesDefault(t *testing.T) {
	t.Setenv(blockCacheMiBEnv, "")
	require.NoError(t, os.Unsetenv(blockCacheMiBEnv))
	capacity, err := blockCacheBytes()
	require.NoError(t, err)
	require.Equal(t, int64(32<<20), capacity)
}

func TestBlockCacheBytesConfigured(t *testing.T) {
	for mib, want := range map[string]int64{"32": 32 << 20, "512": 512 << 20, "2048": 2048 << 20, "4096": 4096 << 20} {
		t.Run(mib, func(t *testing.T) {
			t.Setenv(blockCacheMiBEnv, mib)
			capacity, err := blockCacheBytes()
			require.NoError(t, err)
			require.Equal(t, want, capacity)
			db := newAscendingIterTestDB(t)
			applyVersion(t, db, ascIterTestStore, 5, []byte("fee"), []byte("fee@5"))
			applyVersion(t, db, ascIterTestStore, 10, []byte("fee"), []byte("fee@10"))
			fee, err := db.Get(ascIterTestStore, 5, []byte("fee"))
			require.NoError(t, err)
			require.Equal(t, []byte("fee@5"), fee)
		})
	}
}

func TestBlockCacheBytesRejectsInvalidCapacityBeforeOpening(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "512MiB", "1.5", "8796093022208", "9223372036854775808"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(blockCacheMiBEnv, value)
			dir := filepath.Join(t.TempDir(), "unopened")
			store, err := OpenDB(dir, config.DefaultStateStoreConfig())
			require.ErrorContains(t, err, blockCacheMiBEnv)
			require.Nil(t, store)
			require.NoDirExists(t, dir)
		})
	}
}
