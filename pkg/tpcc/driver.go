package tpcc

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// BenchmarkDriver coordinates concurrent worker goroutines and workload execution
type BenchmarkDriver struct {
	db          *sql.DB
	cfg         Config
	stats       *StatsCollector
	itemCount   int
	custCount   int
	groupCommit *GroupCommitQueue
}

// NewBenchmarkDriver creates a new driver instance
func NewBenchmarkDriver(db *sql.DB, cfg Config) *BenchmarkDriver {
	itemCount := (ItemCount * cfg.ScalePercent) / 100
	if itemCount < 100 {
		itemCount = 100
	}
	custCount := (CustomersPerDist * cfg.ScalePercent) / 100
	if custCount < 10 {
		custCount = 10
	}

	var gc *GroupCommitQueue
	if cfg.GroupCommit {
		gc = NewGroupCommitQueue(db, cfg.BatchSize, cfg.BatchTimeout)
	}

	return &BenchmarkDriver{
		db:          db,
		cfg:         cfg,
		stats:       NewStatsCollector(),
		itemCount:   itemCount,
		custCount:   custCount,
		groupCommit: gc,
	}
}

// Run executes the warmup, benchmark measurement, and reports statistics
func (d *BenchmarkDriver) Run() error {
	syncMode := d.cfg.Synchronous
	if syncMode == "" {
		syncMode = "NORMAL"
	}

	fmt.Printf("\n--- Starting TPC-C Benchmark ---\n")
	fmt.Printf("Database:       %s\n", d.cfg.DBPath)
	fmt.Printf("Warehouses:     %d\n", d.cfg.Warehouses)
	fmt.Printf("Workers:        %d\n", d.cfg.Threads)
	fmt.Printf("Warmup:         %v\n", d.cfg.WarmupTime)
	fmt.Printf("Duration:       %v\n", d.cfg.Duration)
	fmt.Printf("Cache Size:     %d MB\n", d.cfg.CacheSizeMB)
	fmt.Printf("Journal Mode:   WAL (sync: %s)\n", syncMode)
	if d.cfg.GroupCommit {
		fmt.Printf("Group Commit:   ENABLED (batch_size: %d, timeout: %v)\n\n", d.cfg.BatchSize, d.cfg.BatchTimeout)
	} else {
		fmt.Printf("Group Commit:   DISABLED (direct concurrent transactions)\n\n")
	}

	if d.groupCommit != nil {
		defer d.groupCommit.Stop()
	}

	stopChan := make(chan struct{})
	var wg sync.WaitGroup

	// Launch worker goroutines
	for id := 0; id < d.cfg.Threads; id++ {
		wg.Add(1)
		go d.worker(id, stopChan, &wg)
	}

	// Warmup phase
	if d.cfg.WarmupTime > 0 {
		fmt.Printf("Running warmup for %v...\n", d.cfg.WarmupTime)
		time.Sleep(d.cfg.WarmupTime)
	}

	// Start measurement phase
	fmt.Println("Measurement started.")
	d.stats.StartMeasurement()

	// Periodic interval reporter
	ticker := time.NewTicker(d.cfg.ReportInterval)
	defer ticker.Stop()

	measureEnd := time.After(d.cfg.Duration)
	done := false

	for !done {
		select {
		case <-measureEnd:
			done = true
		case <-ticker.C:
			fmt.Println(d.stats.IntervalReport())
		}
	}

	// Stop workers
	close(stopChan)
	wg.Wait()

	// Print final detailed summary
	d.stats.Summary()
	return nil
}

func (d *BenchmarkDriver) worker(id int, stopChan <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	rg := NewRandGen()

	for {
		select {
		case <-stopChan:
			return
		default:
		}

		// Standard TPC-C transaction mix:
		// 45% New-Order, 43% Payment, 4% Order-Status, 4% Delivery, 4% Stock-Level
		roll := rg.IntRange(1, 100)
		var res TxResult
		var err error

		switch {
		case roll <= 45:
			if d.groupCommit != nil {
				res = d.groupCommit.Submit(TxNewOrder, func(tx *sql.Tx) (bool, error) {
					return ExecNewOrder(tx, rg, d.cfg.Warehouses, d.itemCount, d.custCount)
				})
			} else {
				res, err = RunNewOrder(d.db, rg, d.cfg.Warehouses, d.itemCount, d.custCount)
			}
		case roll <= 88:
			if d.groupCommit != nil {
				res = d.groupCommit.Submit(TxPayment, func(tx *sql.Tx) (bool, error) {
					err := ExecPayment(tx, rg, d.cfg.Warehouses, d.custCount)
					return false, err
				})
			} else {
				res, err = RunPayment(d.db, rg, d.cfg.Warehouses, d.custCount)
			}
		case roll <= 92:
			// Read-only: runs concurrently against db
			res, err = RunOrderStatus(d.db, rg, d.cfg.Warehouses, d.custCount)
		case roll <= 96:
			if d.groupCommit != nil {
				res = d.groupCommit.Submit(TxDelivery, func(tx *sql.Tx) (bool, error) {
					err := ExecDelivery(tx, rg, d.cfg.Warehouses)
					return false, err
				})
			} else {
				res, err = RunDelivery(d.db, rg, d.cfg.Warehouses)
			}
		default:
			// Read-only: runs concurrently against db
			res, err = RunStockLevel(d.db, rg, d.cfg.Warehouses)
		}

		if err != nil && res.Err == nil {
			res.Err = err
		}

		d.stats.Record(res)
	}
}
