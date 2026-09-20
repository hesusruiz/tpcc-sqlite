package tpcc

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// BenchmarkDriver coordinates concurrent worker goroutines and workload execution
type BenchmarkDriver struct {
	db        *sql.DB
	cfg       Config
	stats     *StatsCollector
	itemCount int
	custCount int
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

	return &BenchmarkDriver{
		db:        db,
		cfg:       cfg,
		stats:     NewStatsCollector(),
		itemCount: itemCount,
		custCount: custCount,
	}
}

// Run executes the warmup, benchmark measurement, and reports statistics
func (d *BenchmarkDriver) Run() error {
	fmt.Printf("\n--- Starting TPC-C Benchmark ---\n")
	fmt.Printf("Database:       %s\n", d.cfg.DBPath)
	fmt.Printf("Warehouses:     %d\n", d.cfg.Warehouses)
	fmt.Printf("Workers:        %d\n", d.cfg.Threads)
	fmt.Printf("Warmup:         %v\n", d.cfg.WarmupTime)
	fmt.Printf("Duration:       %v\n", d.cfg.Duration)
	fmt.Printf("Cache Size:     %d MB\n", d.cfg.CacheSizeMB)
	fmt.Printf("Journal Mode:   WAL (sync: NORMAL)\n\n")

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
			res, err = RunNewOrder(d.db, rg, d.cfg.Warehouses, d.itemCount, d.custCount)
		case roll <= 88:
			res, err = RunPayment(d.db, rg, d.cfg.Warehouses, d.custCount)
		case roll <= 92:
			res, err = RunOrderStatus(d.db, rg, d.cfg.Warehouses, d.custCount)
		case roll <= 96:
			res, err = RunDelivery(d.db, rg, d.cfg.Warehouses)
		default:
			res, err = RunStockLevel(d.db, rg, d.cfg.Warehouses)
		}

		if err != nil && res.Err == nil {
			res.Err = err
		}

		d.stats.Record(res)
	}
}
