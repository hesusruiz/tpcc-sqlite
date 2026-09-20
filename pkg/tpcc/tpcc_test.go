package tpcc

import (
	"os"
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

	// 8. Test driver running for a brief moment
	driver := NewBenchmarkDriver(db, cfg)
	if err := driver.Run(); err != nil {
		t.Fatalf("driver.Run failed: %v", err)
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
