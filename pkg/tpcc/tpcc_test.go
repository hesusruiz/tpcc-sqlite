package tpcc

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestTPCCWorkload(t *testing.T) {
	testDBPath := "test_tpcc.db"
	_ = RemoveDBFiles(testDBPath)
	defer RemoveDBFiles(testDBPath)

	cfg := Config{
		DBPath:         testDBPath,
		Warehouses:     1,
		ScalePercent:   2, // 2% scale for fast unit test
		Threads:        2,
		WarmupTime:     100 * time.Millisecond,
		Duration:       500 * time.Millisecond,
		ReportInterval: 200 * time.Millisecond,
		CacheSizeMB:    128,
		BusyTimeoutMS:  5000,
	}

	db, err := OpenDB(cfg)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	// 1. Verify schema creation
	if err := CreateSchema(db, true); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// 2. Verify data loading
	if err := LoadData(db, cfg); err != nil {
		t.Fatalf("failed to load data: %v", err)
	}

	rg := NewRandGen()
	itemCount := (ItemCount * cfg.ScalePercent) / 100
	custCount := (CustomersPerDist * cfg.ScalePercent) / 100

	// 3. Test New-Order
	resNo, err := RunNewOrder(db, rg, cfg.Warehouses, itemCount, custCount)
	if err != nil && !resNo.Rollback {
		t.Fatalf("RunNewOrder failed: %v", err)
	}
	if resNo.Latency < 0 {
		t.Errorf("expected non-negative latency, got %v", resNo.Latency)
	}

	// 4. Test Payment
	resPay, err := RunPayment(db, rg, cfg.Warehouses, custCount)
	if err != nil {
		t.Fatalf("RunPayment failed: %v", err)
	}
	if resPay.Latency < 0 {
		t.Errorf("expected non-negative latency, got %v", resPay.Latency)
	}

	// 5. Test Order-Status
	resOrd, err := RunOrderStatus(db, rg, cfg.Warehouses, custCount)
	if err != nil {
		t.Fatalf("RunOrderStatus failed: %v", err)
	}
	if resOrd.Latency < 0 {
		t.Errorf("expected non-negative latency, got %v", resOrd.Latency)
	}

	// 6. Test Delivery
	resDel, err := RunDelivery(db, rg, cfg.Warehouses)
	if err != nil {
		t.Fatalf("RunDelivery failed: %v", err)
	}
	if resDel.Latency < 0 {
		t.Errorf("expected non-negative latency, got %v", resDel.Latency)
	}

	// 7. Test Stock-Level
	resSlev, err := RunStockLevel(db, rg, cfg.Warehouses)
	if err != nil {
		t.Fatalf("RunStockLevel failed: %v", err)
	}
	if resSlev.Latency < 0 {
		t.Errorf("expected non-negative latency, got %v", resSlev.Latency)
	}

	// 8. Test driver running for a brief moment in standard mode
	driver := NewBenchmarkDriver(db, cfg)
	if err := driver.Run(); err != nil {
		t.Fatalf("driver.Run standard failed: %v", err)
	}

	// 9. Test driver running in group commit mode
	cfgGC := cfg
	cfgGC.GroupCommit = true
	cfgGC.BatchSize = 8
	cfgGC.BatchTimeout = 1 * time.Millisecond
	driverGC := NewBenchmarkDriver(db, cfgGC)
	if err := driverGC.Run(); err != nil {
		t.Fatalf("driver.Run group commit failed: %v", err)
	}
}

func TestGroupCommitQueue(t *testing.T) {
	testDB := "test_gc.db"
	_ = RemoveDBFiles(testDB)
	defer RemoveDBFiles(testDB)

	cfg := Config{
		DBPath:        testDB,
		BusyTimeoutMS: 5000,
		CacheSizeMB:   16,
	}
	db, err := OpenDB(cfg)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE gc_items (id INT PRIMARY KEY, name TEXT);"); err != nil {
		t.Fatalf("create table failed: %v", err)
	}

	gc := NewGroupCommitQueue(db, 4, 2*time.Millisecond)
	defer gc.Stop()

	var wg sync.WaitGroup
	numClients := 10
	results := make([]TxResult, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			res := gc.Submit(TxNewOrder, func(tx *sql.Tx) (bool, error) {
				if idx == 5 {
					// Client 5 simulates rollback
					_, _ = tx.Exec("INSERT INTO gc_items VALUES (?, ?)", 100+idx, fmt.Sprintf("name_%d", idx))
					return true, nil
				}
				_, err := tx.Exec("INSERT INTO gc_items VALUES (?, ?)", idx, fmt.Sprintf("name_%d", idx))
				return false, err
			})
			results[idx] = res
		}()
	}

	wg.Wait()

	// Check results
	for i, res := range results {
		if i == 5 {
			if !res.Rollback {
				t.Errorf("client 5 expected rollback, got %+v", res)
			}
		} else {
			if res.Err != nil {
				t.Errorf("client %d failed: %v", i, res.Err)
			}
			if res.Rollback {
				t.Errorf("client %d unexpectedly rolled back", i)
			}
		}
	}

	// Verify row count in database (9 rows committed, client 5 rolled back)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM gc_items;").Scan(&count); err != nil {
		t.Fatalf("query count failed: %v", err)
	}
	if count != 9 {
		t.Fatalf("expected 9 committed rows, got %d", count)
	}
}

func TestStatsCollector(t *testing.T) {
	sc := NewStatsCollector()
	sc.StartMeasurement()

	for i := 0; i < 100; i++ {
		sc.Record(TxResult{
			TxType:  TxNewOrder,
			Latency: time.Duration(i+1) * time.Millisecond,
		})
	}

	report := sc.IntervalReport()
	if report == "" {
		t.Fatal("expected non-empty interval report")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
