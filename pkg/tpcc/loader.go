package tpcc

import (
	"database/sql"
	"fmt"
	"time"
)

// LoadData populates the database with initial TPC-C data
func LoadData(db *sql.DB, cfg Config) error {
	start := time.Now()
	rg := NewRandGen()

	itemCount := (ItemCount * cfg.ScalePercent) / 100
	if itemCount < 100 {
		itemCount = 100
	}
	custCount := (CustomersPerDist * cfg.ScalePercent) / 100
	if custCount < 10 {
		custCount = 10
	}
	ordersCount := (OrdersPerDist * cfg.ScalePercent) / 100
	if ordersCount < 10 {
		ordersCount = 10
	}

	totalCust := custCount * DistrictsPerWh * cfg.Warehouses
	totalOrders := ordersCount * DistrictsPerWh * cfg.Warehouses
	totalStock := itemCount * cfg.Warehouses

	fmt.Printf("\n--- Populating TPC-C Database ---\n")
	fmt.Printf("Warehouses:          %d\n", cfg.Warehouses)
	fmt.Printf("Districts:           %d (%d per warehouse)\n", DistrictsPerWh*cfg.Warehouses, DistrictsPerWh)
	fmt.Printf("Items:               %d (Scale: %d%%)\n", itemCount, cfg.ScalePercent)
	fmt.Printf("Customers/District:  %d (Total: %d)\n", custCount, totalCust)
	fmt.Printf("Orders/District:     %d (Total: %d)\n", ordersCount, totalOrders)
	fmt.Printf("Stock/Warehouse:     %d (Total: %d)\n", itemCount, totalStock)

	// 1. Load Items
	fmt.Print("Loading items... ")
	if err := loadItems(db, rg, itemCount); err != nil {
		return fmt.Errorf("failed to load items: %w", err)
	}
	fmt.Println("Done.")

	// 2. Load Warehouses and related tables
	for w := 1; w <= cfg.Warehouses; w++ {
		fmt.Printf("Loading warehouse %d of %d... ", w, cfg.Warehouses)
		if err := loadWarehouse(db, rg, w); err != nil {
			return fmt.Errorf("failed to load warehouse %d: %w", w, err)
		}

		if err := loadStock(db, rg, w, itemCount); err != nil {
			return fmt.Errorf("failed to load stock for warehouse %d: %w", w, err)
		}

		for d := 1; d <= DistrictsPerWh; d++ {
			if err := loadDistrict(db, rg, w, d, ordersCount); err != nil {
				return fmt.Errorf("failed to load district %d, warehouse %d: %w", d, w, err)
			}

			if err := loadCustomersAndHistory(db, rg, w, d, custCount); err != nil {
				return fmt.Errorf("failed to load customers for district %d, warehouse %d: %w", d, w, err)
			}

			if err := loadOrders(db, rg, w, d, ordersCount, custCount, itemCount); err != nil {
				return fmt.Errorf("failed to load orders for district %d, warehouse %d: %w", d, w, err)
			}
		}
		fmt.Println("Done.")
	}

	fmt.Printf("Data population completed in %v.\n\n", time.Since(start).Round(time.Millisecond))
	return nil
}

func loadItems(db *sql.DB, rg *RandGen, count int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO item (i_id, i_im_id, i_name, i_price, i_data) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for id := 1; id <= count; id++ {
		imID := rg.IntRange(1, 10000)
		name := rg.AString(14, 24)
		price := rg.FloatRange(1.00, 100.00, 2)
		data := rg.DataWithOriginal(26, 50)

		if _, err := stmt.Exec(id, imID, name, price, data); err != nil {
			return err
		}

		// Commit in batches of 10,000 to keep memory low
		if id%10000 == 0 && id < count {
			if err := tx.Commit(); err != nil {
				return err
			}
			tx, err = db.Begin()
			if err != nil {
				return err
			}
			stmt, err = tx.Prepare(`INSERT INTO item (i_id, i_im_id, i_name, i_price, i_data) VALUES (?, ?, ?, ?, ?)`)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func loadWarehouse(db *sql.DB, rg *RandGen, wID int) error {
	query := `INSERT INTO warehouse (w_id, w_name, w_street_1, w_street_2, w_city, w_state, w_zip, w_tax, w_ytd)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	name := rg.AString(6, 10)
	s1 := rg.AString(10, 20)
	s2 := rg.AString(10, 20)
	city := rg.AString(10, 20)
	state := rg.StateString()
	zip := rg.ZipString()
	tax := rg.FloatRange(0.0000, 0.2000, 4)
	ytd := 300000.00

	_, err := db.Exec(query, wID, name, s1, s2, city, state, zip, tax, ytd)
	return err
}

func loadStock(db *sql.DB, rg *RandGen, wID int, itemCount int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `INSERT INTO stock (s_i_id, s_w_id, s_quantity,
		s_dist_01, s_dist_02, s_dist_03, s_dist_04, s_dist_05,
		s_dist_06, s_dist_07, s_dist_08, s_dist_09, s_dist_10,
		s_ytd, s_order_cnt, s_remote_cnt, s_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for id := 1; id <= itemCount; id++ {
		qty := rg.IntRange(10, 100)
		d01 := rg.AString(24, 24)
		d02 := rg.AString(24, 24)
		d03 := rg.AString(24, 24)
		d04 := rg.AString(24, 24)
		d05 := rg.AString(24, 24)
		d06 := rg.AString(24, 24)
		d07 := rg.AString(24, 24)
		d08 := rg.AString(24, 24)
		d09 := rg.AString(24, 24)
		d10 := rg.AString(24, 24)
		data := rg.DataWithOriginal(26, 50)

		if _, err := stmt.Exec(id, wID, qty, d01, d02, d03, d04, d05, d06, d07, d08, d09, d10, 0.0, 0, 0, data); err != nil {
			return err
		}

		if id%10000 == 0 && id < itemCount {
			if err := tx.Commit(); err != nil {
				return err
			}
			tx, err = db.Begin()
			if err != nil {
				return err
			}
			stmt, err = tx.Prepare(query)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func loadDistrict(db *sql.DB, rg *RandGen, wID, dID, ordersCount int) error {
	query := `INSERT INTO district (d_id, d_w_id, d_name, d_street_1, d_street_2, d_city, d_state, d_zip, d_tax, d_ytd, d_next_o_id)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	name := rg.AString(6, 10)
	s1 := rg.AString(10, 20)
	s2 := rg.AString(10, 20)
	city := rg.AString(10, 20)
	state := rg.StateString()
	zip := rg.ZipString()
	tax := rg.FloatRange(0.0000, 0.2000, 4)
	ytd := 30000.00
	nextOID := ordersCount + 1

	_, err := db.Exec(query, dID, wID, name, s1, s2, city, state, zip, tax, ytd, nextOID)
	return err
}

func loadCustomersAndHistory(db *sql.DB, rg *RandGen, wID, dID, custCount int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	cStmt, err := tx.Prepare(`INSERT INTO customer (c_id, c_d_id, c_w_id, c_first, c_middle, c_last,
		c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_since,
		c_credit, c_credit_lim, c_discount, c_balance, c_ytd_payment,
		c_payment_cnt, c_delivery_cnt, c_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer cStmt.Close()

	hStmt, err := tx.Prepare(`INSERT INTO history (h_c_id, h_c_d_id, h_c_w_id, h_d_id, h_w_id, h_date, h_amount, h_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer hStmt.Close()

	now := time.Now().Format("2006-01-02 15:04:05")

	for cID := 1; cID <= custCount; cID++ {
		first := rg.AString(8, 16)
		middle := "OE"
		var last string
		if cID <= 1000 {
			last = MakeLastName(cID - 1)
		} else {
			last = rg.NURandLastName()
		}

		s1 := rg.AString(10, 20)
		s2 := rg.AString(10, 20)
		city := rg.AString(10, 20)
		state := rg.StateString()
		zip := rg.ZipString()
		phone := rg.NString(16, 16)

		credit := "GC"
		if rg.IntRange(1, 10) == 1 {
			credit = "BC"
		}
		lim := 50000.00
		disc := rg.FloatRange(0.0000, 0.5000, 4)
		bal := -10.00
		ytd := 10.00
		pCnt := 1
		dCnt := 0
		data := rg.AString(300, 500)

		if _, err := cStmt.Exec(cID, dID, wID, first, middle, last, s1, s2, city, state, zip, phone, now,
			credit, lim, disc, bal, ytd, pCnt, dCnt, data); err != nil {
			return err
		}

		hData := rg.AString(12, 24)
		if _, err := hStmt.Exec(cID, dID, wID, dID, wID, now, 10.00, hData); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func loadOrders(db *sql.DB, rg *RandGen, wID, dID, ordersCount, custCount, itemCount int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	oStmt, err := tx.Prepare(`INSERT INTO orders (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_carrier_id, o_ol_cnt, o_all_local)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer oStmt.Close()

	noStmt, err := tx.Prepare(`INSERT INTO new_orders (no_o_id, no_d_id, no_w_id) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer noStmt.Close()

	olStmt, err := tx.Prepare(`INSERT INTO order_line (ol_o_id, ol_d_id, ol_w_id, ol_number, ol_i_id, ol_supply_w_id, ol_delivery_d, ol_quantity, ol_amount, ol_dist_info)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer olStmt.Close()

	now := time.Now().Format("2006-01-02 15:04:05")

	// Permutation of customer IDs for orders
	custPerm := make([]int, ordersCount)
	for i := 0; i < ordersCount; i++ {
		custPerm[i] = (i % custCount) + 1
	}
	for i := ordersCount - 1; i > 0; i-- {
		j := rg.IntRange(0, i)
		custPerm[i], custPerm[j] = custPerm[j], custPerm[i]
	}

	limitNewOrders := (ordersCount * 7) / 10 // e.g. 2100 out of 3000

	for oID := 1; oID <= ordersCount; oID++ {
		cID := custPerm[oID-1]
		carrierID := 0
		if oID <= limitNewOrders {
			carrierID = rg.IntRange(1, 10)
		}
		olCnt := rg.IntRange(5, 15)

		if _, err := oStmt.Exec(oID, dID, wID, cID, now, carrierID, olCnt, 1); err != nil {
			return err
		}

		if oID > limitNewOrders {
			if _, err := noStmt.Exec(oID, dID, wID); err != nil {
				return err
			}
		}

		for olNum := 1; olNum <= olCnt; olNum++ {
			itemID := rg.IntRange(1, itemCount)
			deliveryD := sql.NullString{String: now, Valid: oID <= limitNewOrders}
			amount := 0.00
			if oID > limitNewOrders {
				amount = rg.FloatRange(0.01, 9999.99, 2)
			}
			distInfo := rg.AString(24, 24)

			if _, err := olStmt.Exec(oID, dID, wID, olNum, itemID, wID, deliveryD, 5, amount, distInfo); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}
