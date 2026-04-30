package migrator

import "time"

func nowISOImpl() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
