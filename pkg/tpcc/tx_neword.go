package tpcc

import (
	"database/sql"
	"errors"
	"time"
)

// ErrInvalidItem simulates the TPC-C 1% invalid item rollback
var ErrInvalidItem = errors.New("tpcc: invalid item id, rolled back")

// RunNewOrder executes the TPC-C New-Order transaction
func RunNewOrder(db *sql.DB, rg *RandGen, maxW int, itemCount, custCount int) (TxResult, error) {
	start := time.Now()

	wID := rg.IntRange(1, maxW)
	dID := rg.IntRange(1, DistrictsPerWh)
	cID := rg.NURandCID(custCount)
	olCnt := rg.IntRange(5, 15)
	rbk := rg.IntRange(1, 100) // 1% rollback

	type orderItem struct {
		itemID    int
		supplyWID int
		quantity  int
	}

	items := make([]orderItem, olCnt)
	allLocal := 1

	for i := 0; i < olCnt; i++ {
		items[i].itemID = rg.NURandItemID(itemCount)
		// 1% of transactions use an unused item ID for the last item to test rollback
		if i == olCnt-1 && rbk == 1 {
			items[i].itemID = itemCount + 100 // guaranteed invalid
		}

		if maxW > 1 && rg.IntRange(1, 100) == 1 {
			// Remote warehouse
			items[i].supplyWID = rg.IntRange(1, maxW)
			if items[i].supplyWID != wID {
				allLocal = 0
			}
		} else {
			items[i].supplyWID = wID
		}
		items[i].quantity = rg.IntRange(1, 10)
	}

	tx, err := db.Begin()
	if err != nil {
		return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
	}
	defer tx.Rollback()

	// 1. Get customer discount, last name, credit, and warehouse tax
	var cDiscount, wTax float64
	var cLast, cCredit string
	qCust := `SELECT c_discount, c_last, c_credit, w_tax
	          FROM customer, warehouse
	          WHERE w_id = ? AND c_w_id = ? AND c_d_id = ? AND c_id = ?`
	err = tx.QueryRow(qCust, wID, wID, dID, cID).Scan(&cDiscount, &cLast, &cCredit, &wTax)
	if err != nil {
		return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
	}

	// 2. Get and increment district next_o_id
	var dNextOID int
	var dTax float64
	qDist := `SELECT d_next_o_id, d_tax FROM district WHERE d_id = ? AND d_w_id = ?`
	if err := tx.QueryRow(qDist, dID, wID).Scan(&dNextOID, &dTax); err != nil {
		return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
	}

	oID := dNextOID
	_, err = tx.Exec(`UPDATE district SET d_next_o_id = d_next_o_id + 1 WHERE d_id = ? AND d_w_id = ?`, dID, wID)
	if err != nil {
		return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
	}

	// 3. Insert into orders and new_orders
	now := time.Now().Format("2006-01-02 15:04:05")
	qOrder := `INSERT INTO orders (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_carrier_id, o_ol_cnt, o_all_local)
	           VALUES (?, ?, ?, ?, ?, 0, ?, ?)`
	if _, err := tx.Exec(qOrder, oID, dID, wID, cID, now, olCnt, allLocal); err != nil {
		return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
	}

	if _, err := tx.Exec(`INSERT INTO new_orders (no_o_id, no_d_id, no_w_id) VALUES (?, ?, ?)`, oID, dID, wID); err != nil {
		return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
	}

	// Prepare order line statement
	olStmt, err := tx.Prepare(`INSERT INTO order_line (ol_o_id, ol_d_id, ol_w_id, ol_number, ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_dist_info)
	                           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
	}
	defer olStmt.Close()

	// 4. Process each item
	for i, item := range items {
		var iPrice float64
		var iName, iData string
		err := tx.QueryRow(`SELECT i_price, i_name, i_data FROM item WHERE i_id = ?`, item.itemID).Scan(&iPrice, &iName, &iData)
		if err == sql.ErrNoRows {
			// Item not found: trigger intended rollback
			tx.Rollback()
			return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Rollback: true}, nil
		}
		if err != nil {
			return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
		}

		// Query stock
		var sQuantity int
		var sDist01, sDist02, sDist03, sDist04, sDist05 string
		var sDist06, sDist07, sDist08, sDist09, sDist10 string
		qStock := `SELECT s_quantity, s_dist_01, s_dist_02, s_dist_03, s_dist_04, s_dist_05,
		                  s_dist_06, s_dist_07, s_dist_08, s_dist_09, s_dist_10
		           FROM stock WHERE s_i_id = ? AND s_w_id = ?`
		err = tx.QueryRow(qStock, item.itemID, item.supplyWID).Scan(
			&sQuantity, &sDist01, &sDist02, &sDist03, &sDist04, &sDist05,
			&sDist06, &sDist07, &sDist08, &sDist09, &sDist10,
		)
		if err != nil {
			return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
		}

		if sQuantity >= item.quantity+10 {
			sQuantity -= item.quantity
		} else {
			sQuantity = sQuantity - item.quantity + 91
		}

		remoteInc := 0
		if item.supplyWID != wID {
			remoteInc = 1
		}

		_, err = tx.Exec(`UPDATE stock SET s_quantity = ?, s_order_cnt = s_order_cnt + 1, s_remote_cnt = s_remote_cnt + ?
		                  WHERE s_i_id = ? AND s_w_id = ?`, sQuantity, remoteInc, item.itemID, item.supplyWID)
		if err != nil {
			return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
		}

		distInfo := sDist01
		switch dID {
		case 1:
			distInfo = sDist01
		case 2:
			distInfo = sDist02
		case 3:
			distInfo = sDist03
		case 4:
			distInfo = sDist04
		case 5:
			distInfo = sDist05
		case 6:
			distInfo = sDist06
		case 7:
			distInfo = sDist07
		case 8:
			distInfo = sDist08
		case 9:
			distInfo = sDist09
		case 10:
			distInfo = sDist10
		}

		olAmount := float64(item.quantity) * iPrice * (1.0 + wTax + dTax) * (1.0 - cDiscount)
		if _, err := olStmt.Exec(oID, dID, wID, i+1, item.itemID, item.supplyWID, item.quantity, olAmount, distInfo); err != nil {
			return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return TxResult{TxType: TxNewOrder, Latency: time.Since(start), Err: err}, err
	}

	return TxResult{TxType: TxNewOrder, Latency: time.Since(start)}, nil
}
