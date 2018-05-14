package sqlx

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"

	sqlxx "github.com/jmoiron/sqlx"
)

type DB struct {
	*sqlxx.DB
	pool sync.Pool

	rollbacked *rollbacked
	activeTx   *activeTx
}

type Txm struct {
	*sqlxx.Tx

	rollbacked *rollbacked
	activeTx   *activeTx
}

type activeTx struct {
	count uint64
}

type rollbacked struct {
	count uint64
}

func Open(driverName, dataSourceName string) (*DB, error) {
	db, err := sqlxx.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	return &DB{DB: db, activeTx: &activeTx{}}, err
}

func MustOpen(driverName, dataSourceName string) *DB {
	db, err := Open(driverName, dataSourceName)
	if err != nil {
		panic(err)
	}
	return db
}

func (db *DB) Close() error { return db.DB.Close() }

func (db *DB) setTx(tx *sqlxx.Tx) {
	db.pool.Put(&Txm{Tx: tx, activeTx: db.activeTx})
}

func (db *DB) getTxm() *Txm {
	db.activeTx.incrementTx()
	return db.pool.Get().(*Txm)
}

func (db *DB) BeginTxm() (*Txm, error) {
	if !db.activeTx.HasActiveTx() {
		tx, err := db.DB.Beginx()
		if err != nil {
			return nil, err
		}
		db.setTx(tx)
		return db.getTxm(), nil
	}
	return db.getTxm(), new(NestedBeginTxErr)
}

func (db *DB) MustBeginTxm() *Txm {
	txm, err := db.BeginTxm()
	if e, ok := err.(*NestedBeginTxErr); !ok && e != nil {
		panic(err)
	}
	return txm
}

func (db *DB) BeginTxxm(ctx context.Context, opts *sql.TxOptions) (*Txm, error) {
	if !db.activeTx.HasActiveTx() {
		tx, err := db.BeginTxx(ctx, opts)
		if err != nil {
			return nil, err
		}
		db.setTx(tx)
		return db.getTxm(), nil
	}
	return db.getTxm(), new(NestedBeginTxErr)
}

func (db *DB) MustBeginTxxm(ctx context.Context, opts *sql.TxOptions) (*Txm, error) {
	txm, err := db.BeginTxxm(ctx, opts)
	if e, ok := err.(*NestedBeginTxErr); !ok && e != nil {
		panic(err)
	}
	return txm, nil
}

func (t *Txm) Commit() error {
	t.activeTx.decrementTx()
	if t.activeTx.HasActiveTx() {
		return nil
	}
	return t.Tx.Commit()
}

func (t *Txm) Rollback() error {
	if t.activeTx.HasActiveTx() {
		t.activeTx.decrementTx()
		return nil
	}
	return t.Tx.Rollback()
}

func (r *rollbacked) String() string {
	return fmt.Sprintf("rollbacked in nested transaction: %d", r.times())
}

func (r *rollbacked) incrementRollback() {
	atomic.AddUint64(&r.count, 1)
}

func (r *rollbacked) times() uint64 {
	return atomic.LoadUint64(&r.count)
}

func (r *rollbacked) IsRollbacked() bool {
	return r.times() > 0
}

func (a *activeTx) String() string {
	return fmt.Sprintf("active tx counter: %d", a.getActiveTx())
}

func (a *activeTx) incrementTx() {
	atomic.AddUint64(&a.count, 1)
}

func (a *activeTx) decrementTx() {
	if a.HasActiveTx() {
		atomic.AddUint64(&a.count, ^uint64(0))
	}
}

func (a *activeTx) getActiveTx() uint64 {
	return atomic.LoadUint64(&a.count)
}

func (a *activeTx) HasActiveTx() bool {
	return a.getActiveTx() > 0
}
