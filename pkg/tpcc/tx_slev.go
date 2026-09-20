package tpcc

import (
	"database/sql"
	"time"
)

// RunStockLevel executes the TPC-C Stock-Level transaction (read-only)
func RunStockLevel(db *sql.DB, rg *RandGen, maxW int) (TxResult, error) {
	start := time.Now()

	wID := rg.IntRange(1, maxW)
	dID := rg.IntRange(1, DistrictsPerWh)
	threshold := rg.IntRange(10, 20)

	// 1. Get next order ID
	var dNextOID int
	err := db.QueryRow(`SELECT d_next_o_id FROM district WHERE d_id = ? AND d_w_id = ?`, dID, wID).
		Scan(&dNextOID)
	if err != nil {
		return TxResult{TxType: TxStockLevel, Latency: time.Since(start), Err: err}, err
	}

	// 2. Count distinct items in last 20 orders whose stock quantity < threshold
	query := `SELECT count(*)
	          FROM stock
	          WHERE s_w_id = ?
	            AND s_quantity < ?
	            AND s_i_id IN (
	                SELECT DISTINCT ol_i_id
	                FROM order_line
	                WHERE ol_w_id = ?
	                  AND ol_d_id = ?
	                  AND ol_o_id < ?
	                  AND ol_o_id >= ?
	            )`

	var count int
	minOID := dNextOID - 20
	if minOID < 1 {
		minOID = 1
	}

	err = db.QueryRow(query, wID, threshold, wID, dID, dNextOID, minOID).Scan(&count)
	if err != nil {
		return TxResult{TxType: TxStockLevel, Latency: time.Since(start), Err: err}, err
	}

	return TxResult{TxType: TxStockLevel, Latency: time.Since(start)}, nil
}
