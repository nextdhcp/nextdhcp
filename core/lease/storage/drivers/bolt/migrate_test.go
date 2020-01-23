package bolt

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func copyFile(srcFile string) (*os.File, error) {
	file, err := ioutil.TempFile("", "storage-*.db")
	if err != nil {
		return nil, err
	}

	src, err := os.Open(srcFile)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	_, err = io.Copy(file, src)
	if err != nil {
		return nil, err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func TestMigrateDb(t *testing.T) {
	f, err := copyFile("testdata/leases.db")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	db, err := bbolt.Open("testdata/leases.db", 0600, &bbolt.Options{
		OpenFile: func(_ string, _ int, _ os.FileMode) (*os.File, error) {
			return f, nil
		},
	})
	require.NoError(t, err)

	err = migrateDatabase(db)
	assert.NoError(t, err)

	s := &Storage{db: db}
	ips, err := s.ListIPs(context.Background())
	assert.NoError(t, err)
	ids, err := s.ListIDs(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, len(ips), len(ids))

	// we should not have migrated any expired reservation
	countLeased := 0
	for _, ip := range ips {
		cli, leased, expired, err := s.FindByIP(context.Background(), ip)
		assert.NoError(t, err)
		assert.NotEqual(t, "", cli)

		if expired.Before(time.Now()) {
			assert.True(t, leased)
		}

		if leased {
			countLeased++
		}

		// we must also find the same entry via it's ID
		refIP, refLeased, refExpired, refErr := s.FindByID(context.Background(), cli)
		require.NoError(t, refErr, "ip=%q cli=%q leased=%v expired=%q", ip, cli, leased, expired)
		assert.Equal(t, leased, refLeased)
		assert.Equal(t, expired.Unix(), refExpired.Unix())
		assert.True(t, ip.Equal(refIP))
	}

	assert.Equal(t, 7, countLeased)
}
