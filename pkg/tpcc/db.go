package tpcc

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"

	_ "modernc.org/sqlite"
)

// OpenDB opens a SQLite database with WAL mode and performance pragmas configured
func OpenDB(cfg Config) (*sql.DB, error) {
	// Construct DSN with query parameters for modernc.org/sqlite
	// cache_size negative value represents kibibytes (KiB), so 128MB = 128 * 1024 = 131072 KiB
	cacheSizeKiB := cfg.CacheSizeMB * 1024
	if cacheSizeKiB <= 0 {
		cacheSizeKiB = 131072 // Default to 128MB
	}

	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", cfg.BusyTimeoutMS))
	q.Add("_pragma", fmt.Sprintf("cache_size(-%d)", cacheSizeKiB))
	q.Add("_pragma", "temp_store(MEMORY)")
	q.Add("_pragma", "mmap_size(268435456)") // 256MB mmap
	q.Add("_pragma", "foreign_keys(OFF)")     // Disabled for benchmark speed
	q.Add("_txlock", "immediate")             // Acquire write locks immediately to avoid deadlock

	dsn := fmt.Sprintf("%s?%s", cfg.DBPath, q.Encode())

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Ensure connection pool fits worker count
	maxConns := cfg.Threads + 2
	if maxConns < 10 {
		maxConns = 10
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)

	// Ping and run initial pragmas to verify connection and journal mode
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// Ensure WAL mode and synchronous are definitely applied to the primary DB
	initPragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		fmt.Sprintf("PRAGMA busy_timeout = %d;", cfg.BusyTimeoutMS),
		fmt.Sprintf("PRAGMA cache_size = -%d;", cacheSizeKiB),
		"PRAGMA temp_store = MEMORY;",
		"PRAGMA foreign_keys = OFF;",
	}

	for _, p := range initPragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to execute pragma '%s': %w", p, err)
		}
	}

	return db, nil
}

// RemoveDBFiles deletes the database and associated WAL/SHM files
func RemoveDBFiles(dbPath string) error {
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	return nil
}
