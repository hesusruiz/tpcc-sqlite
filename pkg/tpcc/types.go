package tpcc

import (
	"time"
)

// Transaction types in TPC-C
const (
	TxNewOrder = iota
	TxPayment
	TxOrderStatus
	TxDelivery
	TxStockLevel
	NumTxTypes
)

// TxNames provides display names for transaction types
var TxNames = [NumTxTypes]string{
	"New-Order",
	"Payment",
	"Order-Status",
	"Delivery",
	"Stock-Level",
}

// Config defines the benchmark configuration parameters
type Config struct {
	DBPath        string        // Path to SQLite database file
	Warehouses    int           // Number of warehouses (W)
	ScalePercent  int           // Scale percentage (1 to 100, default 100 for full TPC-C data size)
	Threads       int           // Number of concurrent client workers
	WarmupTime    time.Duration // Warmup duration before measurement
	Duration      time.Duration // Duration of benchmark measurement
	ReportInterval time.Duration // Interval for reporting stats (default 2s)
	CacheSizeMB   int           // SQLite cache size in MB (default 128MB)
	BusyTimeoutMS int           // SQLite busy timeout in ms (default 10000ms)
	DropExisting  bool          // Drop existing tables before loading
	LoadOnly      bool          // Only run data loading and exit
	RunOnly       bool          // Only run benchmark (assume DB loaded)
}

// DefaultConfig returns reasonable defaults for SQLite TPC-C
func DefaultConfig() Config {
	return Config{
		DBPath:         "tpcc.db",
		Warehouses:     1,
		ScalePercent:   100,
		Threads:        4,
		WarmupTime:     5 * time.Second,
		Duration:       30 * time.Second,
		ReportInterval: 2 * time.Second,
		CacheSizeMB:    128,
		BusyTimeoutMS:  10000,
		DropExisting:   false,
		LoadOnly:       false,
		RunOnly:        false,
	}
}

// Table row size constants according to standard TPC-C spec (1 warehouse scale)
const (
	ItemCount        = 100000
	DistrictsPerWh   = 10
	CustomersPerDist = 3000
	StockPerWh       = 100000
	OrdersPerDist    = 3000
)
