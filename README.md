# TPC-C Benchmark for SQLite in Pure Go

An Online Transaction Processing (OLTP) benchmark for SQLite written in pure Go using [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite) (no CGO required).

Inspired by the standard [TPC-C Specification](http://www.tpc.org/tpcc/) and the C implementation [rohankadekodi/tpcc-sqlite](https://github.com/rohankadekodi/tpcc-sqlite).

---

## Key Features

- **Pure Go**: Built with `modernc.org/sqlite`, fully cross-platform with zero C compiler or CGO dependencies.
- **SQLite Pragmas**:
  - `PRAGMA journal_mode = WAL;` (Write-Ahead Logging for concurrent readers & writer)
  - `PRAGMA synchronous = NORMAL;` (Minimal fsync overhead on commits while preserving normal WAL durability)
  - `PRAGMA cache_size = -131072;` (128 MB RAM page cache)
  - `PRAGMA busy_timeout = 10000;` (10s busy wait timeout for handling lock contention)
  - `PRAGMA temp_store = MEMORY;`
  - `_txlock=immediate` (Acquires write locks upfront using `BEGIN IMMEDIATE` to prevent deadlock and lock-upgrade race conditions)
- **All 5 TPC-C Transaction Profiles**:
  1. **New-Order** (~45%): Read-write order creation, stock quantity decrement, line item calculation, and ~1% intended user rollback.
  2. **Payment** (~43%): Read-write balance update on warehouse, district, and customer (by ID or last name), plus history logging.
  3. **Order-Status** (~4%): Read-only status inquiry for a customer's most recent order and line items.
  4. **Delivery** (~4%): Read-write batch processing of oldest unfulfilled orders across all 10 districts.
  5. **Stock-Level** (~4%): Read-only district stock count scan for low-inventory items in recent orders.
- **Data Loader**:
  - Batched multi-row inserts inside transactions. Full 1-warehouse scale (~94 MB database):
    - **100,000 items** in the catalog table (`item`).
    - **10 districts** per warehouse (`district`).
    - **3,000 customers per district** = **30,000 customers** per warehouse (`customer` & `history`).
    - **100,000 stock records** per warehouse (`stock`, 1 per item).
    - **3,000 orders per district** = **30,000 orders** per warehouse (`orders`, `new_orders`, and ~300,000 `order_line` records).
  - Configurable `-scale` percentage (e.g. `-scale 10` for fast 10% scale iteration or `-scale 100` for standard).
- **Metrics**:
  - Live interval reporting (TPS, tpmC, P95, P99).
  - Summary metrics report with Min, Avg, P50, P95, P99, Max, and error counts.
  - Standard **tpmC** metric (New-Order transactions per minute).

---

## Installation & Build

For Linux:

```bash
git clone https://github.com/<YOUR_USERNAME>/tpcc-sqlite
cd tpcc-sqlite
go build -o tpcc-sqlite .
```

For Windows, build to an .exe file
```bash
go build -o tpcc-sqlite.exe .
```

---

## Usage

### Quick Start (Prepare & Benchmark)
```bash
# Populate 1 warehouse and run a 30s benchmark with 4 workers
./tpcc-sqlite -w 1 -c 4 -duration 30s -warmup 5s
```

### Separate Loading and Running
```bash
# 1. Load data only (Full 100% scale)
./tpcc-sqlite -load-only -reset -db tpcc.db -w 1 -scale 100

# 2. Run benchmark against loaded database
./tpcc-sqlite -run-only -db tpcc.db -w 1 -c 4 -warmup 5s -duration 60s
```

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-db` | `tpcc.db` | Path to SQLite database file |
| `-w`, `-warehouses` | `1` | Number of warehouses |
| `-scale` | `100` | Scale percentage (1–100) |
| `-c`, `-threads` | `4` | Number of concurrent client workers (goroutines) |
| `-warmup` | `5s` | Warmup duration (e.g. `5s`, `1m`) |
| `-t`, `-duration` | `30s` | Benchmark duration (e.g. `30s`, `5m`) |
| `-interval` | `2s` | Live progress reporting interval |
| `-cache-mb` | `128` | SQLite cache size in MB (default: 128 MB) |
| `-busy-timeout` | `10000` | SQLite busy timeout in ms |
| `-load-only` | `false` | Only create schema and load data, then exit |
| `-run-only` | `false` | Run benchmark against existing database |
| `-drop` | `false` | Drop existing tables before loading |
| `-reset` | `false` | Delete database and WAL files before start |

---

## Benchmark Sample Results

Tested on Windows AMD64 with `modernc.org/sqlite`, 1 Warehouse (100% scale, ~94 MB), 4 workers, WAL mode, sync NORMAL, 128 MB cache:

```
--- Starting TPC-C Benchmark ---
Database:       tpcc_full.db
Warehouses:     1
Workers:        4
Warmup:         3s
Duration:       10s
Cache Size:     128 MB
Journal Mode:   WAL (sync: NORMAL)

Running warmup for 3s...
Measurement started.
[Interval  2.0s] Total TPS:   947.4 | tpmC (NewOrder): 24718.0 | NewOrder P95:   17.44ms | NewOrder P99:   31.37ms
[Interval  2.0s] Total TPS:  1112.6 | tpmC (NewOrder): 29732.0 | NewOrder P95:   13.35ms | NewOrder P99:   31.59ms
[Interval  2.0s] Total TPS:  1023.8 | tpmC (NewOrder): 26996.0 | NewOrder P95:    8.03ms | NewOrder P99:   38.72ms
[Interval  2.0s] Total TPS:   532.5 | tpmC (NewOrder): 13259.6 | NewOrder P95:   27.93ms | NewOrder P99:   95.42ms
[Interval  2.0s] Total TPS:   577.5 | tpmC (NewOrder): 15871.2 | NewOrder P95:    8.70ms | NewOrder P99:  175.53ms

================================================================================
                             TPC-C BENCHMARK RESULTS                            
================================================================================
Elapsed Time:         10.119s
Total Transactions:   8392 (Errors: 0)
Total Throughput:     829.31 TPS
TPC-C Score (tpmC):   21873.27 (New-Order tx/min)
New-Order Ratio:      44.0% (TPC-C spec target: ~45%)
--------------------------------------------------------------------------------
Transaction    |    Count | Errors | Avg (ms) | P50 (ms) | P95 (ms) | P99 (ms)
---------------+----------+--------+----------+----------+----------+----------
New-Order      |     3689 |      0 |     6.99 |     1.23 |    14.78 |    50.10
Payment        |     3739 |      0 |     3.58 |     0.00 |     3.88 |    47.92
Order-Status   |      341 |      0 |     0.22 |     0.00 |     1.00 |     1.56
Delivery       |      312 |      0 |     5.71 |     2.59 |    17.88 |    55.80
Stock-Level    |      311 |      0 |     1.45 |     1.09 |     3.03 |     5.44
================================================================================
```
