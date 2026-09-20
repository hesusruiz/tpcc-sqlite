package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"tpcc-sqlite/pkg/tpcc"
)

func main() {
	cfg := tpcc.DefaultConfig()

	var (
		dbPath       string
		warehouses   int
		scalePercent int
		threads      int
		warmupStr    string
		durationStr  string
		intervalStr  string
		cacheMB      int
		busyTimeout  int
		loadOnly     bool
		runOnly      bool
		dropExisting bool
		resetDB      bool
	)

	flag.StringVar(&dbPath, "db", cfg.DBPath, "Path to SQLite database file")
	flag.IntVar(&warehouses, "w", cfg.Warehouses, "Number of warehouses")
	flag.IntVar(&warehouses, "warehouses", cfg.Warehouses, "Number of warehouses")
	flag.IntVar(&scalePercent, "scale", cfg.ScalePercent, "Scale percentage (1-100, 100 for full TPC-C data size)")
	flag.IntVar(&threads, "threads", cfg.Threads, "Number of worker threads (goroutines)")
	flag.IntVar(&threads, "c", cfg.Threads, "Number of worker threads (shorthand)")
	flag.StringVar(&warmupStr, "warmup", "5s", "Warmup duration (e.g. 5s, 1m)")
	flag.StringVar(&durationStr, "duration", "30s", "Benchmark measurement duration (e.g. 30s, 2m)")
	flag.StringVar(&durationStr, "t", "30s", "Benchmark measurement duration (shorthand)")
	flag.StringVar(&intervalStr, "interval", "2s", "Progress reporting interval")
	flag.IntVar(&cacheMB, "cache-mb", 128, "SQLite cache size in MB (default 128)")
	flag.IntVar(&busyTimeout, "busy-timeout", 10000, "SQLite busy timeout in ms (default 10000)")
	flag.BoolVar(&loadOnly, "load-only", false, "Only create schema and load data, then exit")
	flag.BoolVar(&runOnly, "run-only", false, "Run benchmark against existing database without loading")
	flag.BoolVar(&dropExisting, "drop", false, "Drop existing tables before loading")
	flag.BoolVar(&resetDB, "reset", false, "Delete existing database file and WAL before start")

	flag.Parse()

	warmupDur, err := time.ParseDuration(warmupStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid warmup duration: %v\n", err)
		os.Exit(1)
	}
	durationDur, err := time.ParseDuration(durationStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid measurement duration: %v\n", err)
		os.Exit(1)
	}
	intervalDur, err := time.ParseDuration(intervalStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid interval duration: %v\n", err)
		os.Exit(1)
	}

	cfg.DBPath = dbPath
	cfg.Warehouses = warehouses
	cfg.ScalePercent = scalePercent
	cfg.Threads = threads
	cfg.WarmupTime = warmupDur
	cfg.Duration = durationDur
	cfg.ReportInterval = intervalDur
	cfg.CacheSizeMB = cacheMB
	cfg.BusyTimeoutMS = busyTimeout
	cfg.LoadOnly = loadOnly
	cfg.RunOnly = runOnly
	cfg.DropExisting = dropExisting

	if resetDB {
		_ = tpcc.RemoveDBFiles(cfg.DBPath)
	}

	// Open DB connection with WAL mode and pragmas
	db, err := tpcc.OpenDB(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Load data if not run-only
	if !cfg.RunOnly {
		if err := tpcc.CreateSchema(db, cfg.DropExisting || resetDB); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating schema: %v\n", err)
			os.Exit(1)
		}

		if err := tpcc.LoadData(db, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading data: %v\n", err)
			os.Exit(1)
		}

		if cfg.LoadOnly {
			fmt.Println("Data loading complete. Exiting (--load-only specified).")
			return
		}
	}

	// Run benchmark
	driver := tpcc.NewBenchmarkDriver(db, cfg)
	if err := driver.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Benchmark run error: %v\n", err)
		os.Exit(1)
	}
}
