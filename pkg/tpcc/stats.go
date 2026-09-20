package tpcc

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// TxResult records the outcome of a single transaction execution
type TxResult struct {
	TxType    int
	Latency   time.Duration
	Err       error
	Rollback  bool
}

// LatencyPercentiles holds summary stats
type LatencyPercentiles struct {
	Count   int64
	Errors  int64
	Min     time.Duration
	Mean    time.Duration
	P50     time.Duration
	P90     time.Duration
	P95     time.Duration
	P99     time.Duration
	Max     time.Duration
}

// StatsCollector collects metrics across workers
type StatsCollector struct {
	mu           sync.Mutex
	startTime    time.Time
	isWarmup     int32 // atomic bool (1 = warming up, 0 = measuring)
	
	// Measurement accumulators
	totalCounts  [NumTxTypes]int64
	totalErrors  [NumTxTypes]int64
	latencies    [NumTxTypes][]time.Duration
	
	// Interval accumulators
	intervalCounts [NumTxTypes]int64
	intervalErrors [NumTxTypes]int64
	intervalLats   [NumTxTypes][]time.Duration
	intervalStart  time.Time
}

// NewStatsCollector creates a metrics collector
func NewStatsCollector() *StatsCollector {
	sc := &StatsCollector{
		intervalStart: time.Now(),
	}
	atomic.StoreInt32(&sc.isWarmup, 1)
	return sc
}

// Record records a completed transaction
func (sc *StatsCollector) Record(res TxResult) {
	if atomic.LoadInt32(&sc.isWarmup) == 1 {
		return // Ignore stats during warmup
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	txType := res.TxType
	if txType < 0 || txType >= NumTxTypes {
		return
	}

	sc.totalCounts[txType]++
	sc.intervalCounts[txType]++

	if res.Err != nil && !res.Rollback {
		sc.totalErrors[txType]++
		sc.intervalErrors[txType]++
	}

	sc.latencies[txType] = append(sc.latencies[txType], res.Latency)
	sc.intervalLats[txType] = append(sc.intervalLats[txType], res.Latency)
}

// StartMeasurement transitions from warmup to active measurement
func (sc *StatsCollector) StartMeasurement() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	atomic.StoreInt32(&sc.isWarmup, 0)
	sc.startTime = time.Now()
	sc.intervalStart = time.Now()

	for i := 0; i < NumTxTypes; i++ {
		sc.totalCounts[i] = 0
		sc.totalErrors[i] = 0
		sc.latencies[i] = sc.latencies[i][:0]
		sc.intervalCounts[i] = 0
		sc.intervalErrors[i] = 0
		sc.intervalLats[i] = sc.intervalLats[i][:0]
	}
}

// IntervalReport returns summary metrics for the elapsed interval and resets interval counters
func (sc *StatsCollector) IntervalReport() string {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(sc.intervalStart)
	sc.intervalStart = now

	var totalIntCount int64
	for i := 0; i < NumTxTypes; i++ {
		totalIntCount += sc.intervalCounts[i]
	}

	tps := float64(totalIntCount) / elapsed.Seconds()
	newOrdCount := sc.intervalCounts[TxNewOrder]
	tpmC := float64(newOrdCount) * (60.0 / elapsed.Seconds())

	// Compute percentiles for New-Order during interval
	p := calcPercentiles(sc.intervalCounts[TxNewOrder], sc.intervalErrors[TxNewOrder], sc.intervalLats[TxNewOrder])

	// Reset interval accumulators
	for i := 0; i < NumTxTypes; i++ {
		sc.intervalCounts[i] = 0
		sc.intervalErrors[i] = 0
		sc.intervalLats[i] = sc.intervalLats[i][:0]
	}

	return fmt.Sprintf("[Interval %4.1fs] Total TPS: %7.1f | tpmC (NewOrder): %7.1f | NewOrder P95: %7.2fms | NewOrder P99: %7.2fms",
		elapsed.Seconds(), tps, tpmC,
		float64(p.P95.Microseconds())/1000.0,
		float64(p.P99.Microseconds())/1000.0,
	)
}

// Summary calculates full benchmark results
func (sc *StatsCollector) Summary() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	totalDuration := time.Since(sc.startTime)
	var totalTxCount int64
	var totalErrorCount int64

	for i := 0; i < NumTxTypes; i++ {
		totalTxCount += sc.totalCounts[i]
		totalErrorCount += sc.totalErrors[i]
	}

	tps := float64(totalTxCount) / totalDuration.Seconds()
	newOrderCount := sc.totalCounts[TxNewOrder]
	tpmC := float64(newOrderCount) * (60.0 / totalDuration.Seconds())

	fmt.Println("\n================================================================================")
	fmt.Println("                             TPC-C BENCHMARK RESULTS                            ")
	fmt.Println("================================================================================")
	fmt.Printf("Elapsed Time:         %v\n", totalDuration.Round(time.Millisecond))
	fmt.Printf("Total Transactions:   %d (Errors: %d)\n", totalTxCount, totalErrorCount)
	fmt.Printf("Total Throughput:     %.2f TPS\n", tps)
	fmt.Printf("TPC-C Score (tpmC):   %.2f (New-Order tx/min)\n", tpmC)
	if totalTxCount > 0 {
		fmt.Printf("New-Order Ratio:      %.1f%% (TPC-C spec target: ~45%%)\n", float64(newOrderCount)*100.0/float64(totalTxCount))
	}
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("%-14s | %8s | %6s | %8s | %8s | %8s | %8s\n",
		"Transaction", "Count", "Errors", "Avg (ms)", "P50 (ms)", "P95 (ms)", "P99 (ms)")
	fmt.Println("---------------+----------+--------+----------+----------+----------+----------")

	for i := 0; i < NumTxTypes; i++ {
		p := calcPercentiles(sc.totalCounts[i], sc.totalErrors[i], sc.latencies[i])
		fmt.Printf("%-14s | %8d | %6d | %8.2f | %8.2f | %8.2f | %8.2f\n",
			TxNames[i],
			p.Count,
			p.Errors,
			float64(p.Mean.Microseconds())/1000.0,
			float64(p.P50.Microseconds())/1000.0,
			float64(p.P95.Microseconds())/1000.0,
			float64(p.P99.Microseconds())/1000.0,
		)
	}
	fmt.Println("================================================================================")
}

func calcPercentiles(count, errors int64, lats []time.Duration) LatencyPercentiles {
	if len(lats) == 0 {
		return LatencyPercentiles{Count: count, Errors: errors}
	}

	sorted := make([]time.Duration, len(lats))
	copy(sorted, lats)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, lat := range sorted {
		sum += lat
	}
	mean := sum / time.Duration(len(sorted))

	p := func(pct float64) time.Duration {
		idx := int(math.Ceil(pct*float64(len(sorted)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}

	return LatencyPercentiles{
		Count:  count,
		Errors: errors,
		Min:    sorted[0],
		Mean:   mean,
		P50:    p(0.50),
		P90:    p(0.90),
		P95:    p(0.95),
		P99:    p(0.99),
		Max:    sorted[len(sorted)-1],
	}
}
