package bolt

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"
)

// defaultTimeout is how long the driver waits for the lock of a database
// file that is already open, unless the timeout option says otherwise.
// bbolt itself waits forever, so NextDHCP would hang if two servers used the
// same file.
var defaultTimeout = 5 * time.Second

// defaultMode is the file mode of new database files, unless the mode
// option says otherwise.
const defaultMode os.FileMode = 0o660

func init() {
	storage.MustRegister("bolt", storageFactory)
}

// storageFactory opens a bolt database. Its options are the database file,
// either as the only positional argument or as the "file" option, the
// "timeout" to wait for the lock of the file, and the "mode" of a new file.
func storageFactory(arguments map[string][]string) (storage.LeaseStorage, error) {
	file := ""
	timeout := defaultTimeout
	mode := defaultMode

	for key, values := range arguments {
		switch key {
		case "__args__", "file":
			if len(values) == 0 {
				return nil, fmt.Errorf("no database file configured")
			}

			if len(values) > 1 || file != "" {
				return nil, fmt.Errorf("only one database file can be configured")
			}

			file = values[0]

		case "timeout":
			if len(values) != 1 {
				return nil, fmt.Errorf("timeout must have exactly one value")
			}

			d, err := time.ParseDuration(values[0])
			if err != nil || d < 0 {
				return nil, fmt.Errorf("invalid timeout %q: expected a duration like 10s, or 0 to wait forever", values[0])
			}

			timeout = d

		case "mode":
			if len(values) != 1 {
				return nil, fmt.Errorf("mode must have exactly one value")
			}

			m, err := strconv.ParseUint(strings.TrimPrefix(values[0], "0o"), 8, 32)
			if err != nil || m > 0o777 {
				return nil, fmt.Errorf("invalid mode %q: expected file permissions in octal, like 0600", values[0])
			}

			mode = os.FileMode(m)

		default:
			return nil, fmt.Errorf("unknown option %q", key)
		}
	}

	if file == "" {
		return nil, fmt.Errorf("no database file configured")
	}

	db, err := bbolt.Open(file, mode, &bbolt.Options{Timeout: timeout})
	if errors.Is(err, berrors.ErrTimeout) {
		return nil, fmt.Errorf("database file %s is locked after waiting %s: another server or process uses it", file, timeout)
	}

	if err != nil {
		return nil, err
	}

	d := &Storage{db: db}

	return d, nil
}
