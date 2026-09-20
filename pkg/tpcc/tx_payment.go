package tpcc

import (
	"database/sql"
	"fmt"
	"time"
)

// RunPayment executes the TPC-C Payment transaction
func RunPayment(db *sql.DB, rg *RandGen, maxW, custCount int) (TxResult, error) {
	start := time.Now()

	wID := rg.IntRange(1, maxW)
	dID := rg.IntRange(1, DistrictsPerWh)
	hAmount := rg.FloatRange(1.00, 5000.00, 2)

	var cWID, cDID int
	if maxW > 1 && rg.IntRange(1, 100) <= 15 {
		// 15% remote warehouse
		cWID = rg.IntRange(1, maxW)
		cDID = rg.IntRange(1, DistrictsPerWh)
	} else {
		cWID = wID
		cDID = dID
	}

	byLastName := rg.IntRange(1, 100) <= 60

	tx, err := db.Begin()
	if err != nil {
		return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
	}
	defer tx.Rollback()

	// 1. Update warehouse YTD and fetch name
	if _, err := tx.Exec(`UPDATE warehouse SET w_ytd = w_ytd + ? WHERE w_id = ?`, hAmount, wID); err != nil {
		return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
	}
	var wName string
	if err := tx.QueryRow(`SELECT w_name FROM warehouse WHERE w_id = ?`, wID).Scan(&wName); err != nil {
		return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
	}

	// 2. Update district YTD and fetch name
	if _, err := tx.Exec(`UPDATE district SET d_ytd = d_ytd + ? WHERE d_w_id = ? AND d_id = ?`, hAmount, wID, dID); err != nil {
		return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
	}
	var dName string
	if err := tx.QueryRow(`SELECT d_name FROM district WHERE d_w_id = ? AND d_id = ?`, wID, dID).Scan(&dName); err != nil {
		return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
	}

	// 3. Customer lookup
	var cID int
	if byLastName {
		cLast := rg.NURandLastName()
		rows, err := tx.Query(`SELECT c_id FROM customer WHERE c_w_id = ? AND c_d_id = ? AND c_last = ? ORDER BY c_first`, cWID, cDID, cLast)
		if err != nil {
			return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
		}
		var cIDs []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err == nil {
				cIDs = append(cIDs, id)
			}
		}
		rows.Close()

		if len(cIDs) == 0 {
			// Fallback to random CID if name not found
			cID = rg.NURandCID(custCount)
		} else {
			// TPC-C midpoint rule: (count + 1) / 2 - 1
			cID = cIDs[(len(cIDs)-1)/2]
		}
	} else {
		cID = rg.NURandCID(custCount)
	}

	// 4. Fetch customer details
	var cBalance, cYTDPayment, cCreditLim, cDiscount float64
	var cFirst, cMiddle, cLast, cCredit, cData string
	var cPaymentCnt int
	qCust := `SELECT c_first, c_middle, c_last, c_credit, c_credit_lim, c_discount, c_balance, c_ytd_payment, c_payment_cnt, c_data
	          FROM customer WHERE c_w_id = ? AND c_d_id = ? AND c_id = ?`
	err = tx.QueryRow(qCust, cWID, cDID, cID).Scan(
		&cFirst, &cMiddle, &cLast, &cCredit, &cCreditLim, &cDiscount, &cBalance, &cYTDPayment, &cPaymentCnt, &cData,
	)
	if err != nil {
		return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
	}

	cBalance -= hAmount
	cYTDPayment += hAmount
	cPaymentCnt++

	// 5. Update customer
	if cCredit == "BC" {
		newData := fmt.Sprintf("| %d %d %d %d %d %f %s", cID, cDID, cWID, dID, wID, hAmount, cData)
		if len(newData) > 500 {
			newData = newData[:500]
		}
		_, err = tx.Exec(`UPDATE customer SET c_balance = ?, c_ytd_payment = ?, c_payment_cnt = ?, c_data = ?
		                  WHERE c_w_id = ? AND c_d_id = ? AND c_id = ?`,
			cBalance, cYTDPayment, cPaymentCnt, newData, cWID, cDID, cID)
	} else {
		_, err = tx.Exec(`UPDATE customer SET c_balance = ?, c_ytd_payment = ?, c_payment_cnt = ?
		                  WHERE c_w_id = ? AND c_d_id = ? AND c_id = ?`,
			cBalance, cYTDPayment, cPaymentCnt, cWID, cDID, cID)
	}
	if err != nil {
		return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
	}

	// 6. Insert history record
	now := time.Now().Format("2006-01-02 15:04:05")
	hData := fmt.Sprintf("%-10s    %-10s", wName, dName)
	if len(hData) > 24 {
		hData = hData[:24]
	}
	_, err = tx.Exec(`INSERT INTO history (h_c_id, h_c_d_id, h_c_w_id, h_d_id, h_w_id, h_date, h_amount, h_data)
	                  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		cID, cDID, cWID, dID, wID, now, hAmount, hData)
	if err != nil {
		return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
	}

	if err := tx.Commit(); err != nil {
		return TxResult{TxType: TxPayment, Latency: time.Since(start), Err: err}, err
	}

	return TxResult{TxType: TxPayment, Latency: time.Since(start)}, nil
}
