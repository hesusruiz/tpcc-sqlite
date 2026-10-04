package tpcc

import (
	"database/sql"
	"fmt"
	"time"
)

// WriteTask represents a client request submitted to the group commit queue
type WriteTask struct {
	TxType     int
	Execute    func(tx *sql.Tx) (rolledBack bool, err error)
	StartTime  time.Time
	ResultChan chan TxResult
}

// GroupCommitQueue batches write transactions from multiple concurrent clients into single commits
type GroupCommitQueue struct {
	db           *sql.DB
	taskChan     chan WriteTask
	batchSize    int
	batchTimeout time.Duration
	stopChan     chan struct{}
	doneChan     chan struct{}
}

// NewGroupCommitQueue creates and starts a group commit worker
func NewGroupCommitQueue(db *sql.DB, batchSize int, batchTimeout time.Duration) *GroupCommitQueue {
	if batchSize <= 0 {
		batchSize = 16
	}
	if batchTimeout <= 0 {
		batchTimeout = 1 * time.Millisecond
	}

	q := &GroupCommitQueue{
		db:           db,
		taskChan:     make(chan WriteTask, 2048),
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}

	go q.run()
	return q
}

// Stop shuts down the group commit coordinator and waits for pending tasks to finish
func (q *GroupCommitQueue) Stop() {
	close(q.stopChan)
	<-q.doneChan
}

// Submit sends a transaction to the group commit queue and blocks until it is committed and durable
func (q *GroupCommitQueue) Submit(txType int, exec func(tx *sql.Tx) (bool, error)) TxResult {
	start := time.Now()
	resChan := make(chan TxResult, 1)

	task := WriteTask{
		TxType:     txType,
		Execute:    exec,
		StartTime:  start,
		ResultChan: resChan,
	}

	select {
	case q.taskChan <- task:
		return <-resChan
	case <-q.stopChan:
		return TxResult{
			TxType:  txType,
			Latency: time.Since(start),
			Err:     sql.ErrConnDone,
		}
	}
}

func (q *GroupCommitQueue) run() {
	defer close(q.doneChan)
	batch := make([]WriteTask, 0, q.batchSize)

	for {
		select {
		case <-q.stopChan:
			q.drainRemaining()
			return
		case task := <-q.taskChan:
			batch = append(batch, task)

			// Collect tasks up to batchSize or until batchTimeout expires
			timer := time.NewTimer(q.batchTimeout)
		collectLoop:
			for len(batch) < q.batchSize {
				select {
				case t := <-q.taskChan:
					batch = append(batch, t)
				case <-timer.C:
					break collectLoop
				case <-q.stopChan:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					q.commitBatch(batch)
					q.drainRemaining()
					return
				}
			}

			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}

			q.commitBatch(batch)
			batch = batch[:0]
		}
	}
}

func (q *GroupCommitQueue) drainRemaining() {
	batch := make([]WriteTask, 0, q.batchSize)
	for {
		select {
		case task := <-q.taskChan:
			batch = append(batch, task)
			if len(batch) >= q.batchSize {
				q.commitBatch(batch)
				batch = batch[:0]
			}
		default:
			if len(batch) > 0 {
				q.commitBatch(batch)
			}
			return
		}
	}
}

func (q *GroupCommitQueue) commitBatch(batch []WriteTask) {
	if len(batch) == 0 {
		return
	}

	tx, err := q.db.Begin()
	if err != nil {
		now := time.Now()
		for _, task := range batch {
			task.ResultChan <- TxResult{
				TxType:  task.TxType,
				Latency: now.Sub(task.StartTime),
				Err:     err,
			}
		}
		return
	}

	results := make([]TxResult, len(batch))

	for i, task := range batch {
		spName := fmt.Sprintf("sp_%d", i)
		if _, err := tx.Exec("SAVEPOINT " + spName + ";"); err != nil {
			results[i] = TxResult{TxType: task.TxType, Err: err}
			continue
		}

		rolledBack, execErr := task.Execute(tx)
		if execErr != nil {
			_, _ = tx.Exec("ROLLBACK TO " + spName + ";")
			_, _ = tx.Exec("RELEASE " + spName + ";")
			results[i] = TxResult{TxType: task.TxType, Err: execErr}
		} else if rolledBack {
			_, _ = tx.Exec("ROLLBACK TO " + spName + ";")
			_, _ = tx.Exec("RELEASE " + spName + ";")
			results[i] = TxResult{TxType: task.TxType, Rollback: true}
		} else {
			if _, err := tx.Exec("RELEASE " + spName + ";"); err != nil {
				results[i] = TxResult{TxType: task.TxType, Err: err}
			} else {
				results[i] = TxResult{TxType: task.TxType}
			}
		}
	}

	commitErr := tx.Commit()
	now := time.Now()

	for i, task := range batch {
		res := results[i]
		res.Latency = now.Sub(task.StartTime)
		if commitErr != nil && res.Err == nil && !res.Rollback {
			res.Err = commitErr
		}
		task.ResultChan <- res
	}
}
