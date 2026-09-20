package tpcc

import (
	"database/sql"
	"time"
)

// RunDelivery executes the TPC-C Delivery transaction
func RunDelivery(db *sql.DB, rg *RandGen, maxW int) (TxResult, error) {
	start := time.Now()

	wID := rg.IntRange(1, maxW)
	carrierID := rg.IntRange(1, 10)
	now := time.Now().Format("2006-01-02 15:04:05")

	tx, err := db.Begin()
	if err != nil {
		return TxResult{TxType: TxDelivery, Latency: time.Since(start), Err: err}, err
	}
	defer tx.Rollback()

	for dID := 1; dID <= DistrictsPerWh; dID++ {
		var noOID int
		// 1. Find oldest undelivered order
		err := tx.QueryRow(`SELECT no_o_id FROM new_orders WHERE no_d_id = ? AND no_w_id = ? ORDER BY no_o_id ASC LIMIT 1`, dID, wID).
			Scan(&noOID)
		if err == sql.ErrNoRows || noOID == 0 {
			// No new order for this district; continue to next district
			continue
		}
		if err != nil {
			return TxResult{TxType: TxDelivery, Latency: time.Since(start), Err: err}, err
		}

		// 2. Delete from new_orders
		if _, err := tx.Exec(`DELETE FROM new_orders WHERE no_d_id = ? AND no_w_id = ? AND no_o_id = ?`, dID, wID, noOID); err != nil {
			return TxResult{TxType: TxDelivery, Latency: time.Since(start), Err: err}, err
		}

		// 3. Get customer ID from orders
		var cID int
		if err := tx.QueryRow(`SELECT o_c_id FROM orders WHERE o_d_id = ? AND o_w_id = ? AND o_id = ?`, dID, wID, noOID).Scan(&cID); err != nil {
			return TxResult{TxType: TxDelivery, Latency: time.Since(start), Err: err}, err
		}

		// 4. Update carrier ID in orders
		if _, err := tx.Exec(`UPDATE orders SET o_carrier_id = ? WHERE o_d_id = ? AND o_w_id = ? AND o_id = ?`, carrierID, dID, wID, noOID); err != nil {
			return TxResult{TxType: TxDelivery, Latency: time.Since(start), Err: err}, err
		}

		// 5. Update delivery date in order_line
		if _, err := tx.Exec(`UPDATE order_line SET ol_delivery_d = ? WHERE ol_d_id = ? AND ol_w_id = ? AND ol_o_id = ?`, now, dID, wID, noOID); err != nil {
			return TxResult{TxType: TxDelivery, Latency: time.Since(start), Err: err}, err
		}

		// 6. Sum order amounts
		var totalAmount float64
		err = tx.QueryRow(`SELECT COALESCE(SUM(ol_amount), 0.0) FROM order_line WHERE ol_d_id = ? AND ol_w_id = ? AND ol_o_id = ?`, dID, wID, noOID).
			Scan(&totalAmount)
		if err != nil {
			return TxResult{TxType: TxDelivery, Latency: time.Since(start), Err: err}, err
		}

		// 7. Update customer balance and delivery count
		_, err = tx.Exec(`UPDATE customer SET c_balance = c_balance + ?, c_delivery_cnt = c_delivery_cnt + 1
		                  WHERE c_w_id = ? AND c_d_id = ? AND c_id = ?`, totalAmount, wID, dID, cID)
		if err != nil {
			return TxResult{TxType: TxDelivery, Latency: time.Since(start), Err: err}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return TxResult{TxType: TxDelivery, Latency: time.Since(start), Err: err}, err
	}

	return TxResult{TxType: TxDelivery, Latency: time.Since(start)}, nil
}
