package tpcc

import (
	"database/sql"
	"time"
)

// RunOrderStatus executes the TPC-C Order-Status transaction (read-only)
func RunOrderStatus(db *sql.DB, rg *RandGen, maxW, custCount int) (TxResult, error) {
	start := time.Now()

	wID := rg.IntRange(1, maxW)
	dID := rg.IntRange(1, DistrictsPerWh)
	byLastName := rg.IntRange(1, 100) <= 60

	var cID int
	var cBalance float64
	var cFirst, cMiddle, cLast string

	if byLastName {
		cLast = rg.NURandLastName()
		rows, err := db.Query(`SELECT c_id, c_balance, c_first, c_middle FROM customer
		                       WHERE c_w_id = ? AND c_d_id = ? AND c_last = ? ORDER BY c_first`, wID, dID, cLast)
		if err != nil {
			return TxResult{TxType: TxOrderStatus, Latency: time.Since(start), Err: err}, err
		}

		type custInfo struct {
			id      int
			balance float64
			first   string
			middle  string
		}
		var list []custInfo
		for rows.Next() {
			var ci custInfo
			if err := rows.Scan(&ci.id, &ci.balance, &ci.first, &ci.middle); err == nil {
				list = append(list, ci)
			}
		}
		rows.Close()

		if len(list) == 0 {
			cID = rg.NURandCID(custCount)
			_ = db.QueryRow(`SELECT c_balance, c_first, c_middle, c_last FROM customer
			                 WHERE c_w_id = ? AND c_d_id = ? AND c_id = ?`, wID, dID, cID).
				Scan(&cBalance, &cFirst, &cMiddle, &cLast)
		} else {
			mid := (len(list) - 1) / 2
			cID = list[mid].id
			cBalance = list[mid].balance
			cFirst = list[mid].first
			cMiddle = list[mid].middle
		}
	} else {
		cID = rg.NURandCID(custCount)
		err := db.QueryRow(`SELECT c_balance, c_first, c_middle, c_last FROM customer
		                    WHERE c_w_id = ? AND c_d_id = ? AND c_id = ?`, wID, dID, cID).
			Scan(&cBalance, &cFirst, &cMiddle, &cLast)
		if err != nil && err != sql.ErrNoRows {
			return TxResult{TxType: TxOrderStatus, Latency: time.Since(start), Err: err}, err
		}
	}

	// Find the most recent order for this customer
	var oID int
	var oEntryD string
	var oCarrierID sql.NullInt64
	qOrder := `SELECT o_id, o_entry_d, o_carrier_id FROM orders
	           WHERE o_w_id = ? AND o_d_id = ? AND o_c_id = ?
	           ORDER BY o_id DESC LIMIT 1`
	err := db.QueryRow(qOrder, wID, dID, cID).Scan(&oID, &oEntryD, &oCarrierID)
	if err != nil && err != sql.ErrNoRows {
		return TxResult{TxType: TxOrderStatus, Latency: time.Since(start), Err: err}, err
	}

	if oID > 0 {
		// Fetch order line items
		rows, err := db.Query(`SELECT ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_delivery_d
		                       FROM order_line
		                       WHERE ol_w_id = ? AND ol_d_id = ? AND ol_o_id = ?`, wID, dID, oID)
		if err != nil {
			return TxResult{TxType: TxOrderStatus, Latency: time.Since(start), Err: err}, err
		}
		for rows.Next() {
			var iID, supplyW, qty int
			var amount float64
			var deliveryD sql.NullString
			_ = rows.Scan(&iID, &supplyW, &qty, &amount, &deliveryD)
		}
		rows.Close()
	}

	return TxResult{TxType: TxOrderStatus, Latency: time.Since(start)}, nil
}
