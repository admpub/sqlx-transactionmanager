package sqlx

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	sqlxx "github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

var (
	TestPostgres = true
	TestSqlite   = true
	TestMysql    = true
)

var sldb *DB
var pgdb *DB
var mysqldb *DB
var active = []*sqlxx.DB{}

const indent = "    "

func init() {
	ConnectAll()
}

func ConnectAll() {
	var err error

	pgdsn := os.Getenv("SQLX_POSTGRES_DSN")
	mydsn := os.Getenv("SQLX_MYSQL_DSN")
	sqdsn := os.Getenv("SQLX_SQLITE_DSN")

	TestPostgres = pgdsn != "skip"
	TestMysql = mydsn != "skip"
	TestSqlite = sqdsn != "skip"

	if !strings.Contains(mydsn, "parseTime=true") {
		mydsn += "?parseTime=true"
	}

	if TestPostgres {
		pgdb, err = Open("postgres", pgdsn)
		if err != nil {
			fmt.Printf("Disabling PG tests:\n%s%v\n", indent, err)
			TestPostgres = false
		}
	} else {
		fmt.Println("Disabling Postgres tests")
	}

	if TestMysql {
		mysqldb, err = Open("mysql", mydsn)
		if err != nil {
			fmt.Printf("Disabling MySQL tests:\n%s%v", indent, err)
			TestMysql = false
		}
	} else {
		fmt.Println("Disabling MySQL tests")
	}

	if TestSqlite {
		sldb, err = Open("sqlite3", sqdsn)
		if err != nil {
			fmt.Printf("Disabling SQLite:\n%s%v", indent, err)
			TestSqlite = false
		}
	} else {
		fmt.Println("Disabling SQLite tests")
	}
}

type Schema struct {
	create string
	drop   string
}

func (s Schema) Postgres() (string, string) {
	return s.create, s.drop
}

func (s Schema) MySQL() (string, string) {
	return strings.Replace(s.create, `"`, "`", -1), s.drop
}

func (s Schema) Sqlite3() (string, string) {
	return strings.Replace(s.create, `now()`, `CURRENT_TIMESTAMP`, -1), s.drop
}

var defaultSchema = Schema{
	create: `
CREATE TABLE person (
	first_name text,
	last_name text,
	email text,
	added_at timestamp default now()
);
CREATE TABLE place (
	country text,
	city text NULL,
	telcode integer
);
`,
	drop: `
drop table person;
drop table place;
`,
}

type Person struct {
	FirstName string    `db:"first_name"`
	LastName  string    `db:"last_name"`
	Email     string    `db:"email"`
	AddedAt   time.Time `db:"added_at"`
}

type Place struct {
	Country string
	City    sql.NullString
	TelCode int
}

func TestCommit(t *testing.T) {
	RunWithSchema(defaultSchema, t, func(db *DB, t *testing.T) {
		tx, err := db.BeginTxm()
		if err != nil {
			t.Fatal(err)
		}
		tx.MustExec("INSERT INTO person (first_name, last_name, email) VALUES (?, ?, ?)", "Code", "Hex", "x00.x7f@gmail.com")
		tx.MustExec("UPDATE person SET email = ? WHERE first_name = ? AND last_name = ?", "a@b.com", "Code", "Hex")
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}

		var author Person
		if err := db.Get(&author, "SELECT * FROM person WHERE email = ?", "a@b.com"); err != nil {
			t.Fatal(errors.Wrap(err, db.activeTx.String()))
		}
		if author.FirstName != "Code" || author.LastName != "Hex" {
			t.Fatal("Failed to test commit")
		}
	})
}

func TestRollback(t *testing.T) {
	RunWithSchema(defaultSchema, t, func(db *DB, t *testing.T) {
		tx, err := db.BeginTxm()
		if err != nil {
			t.Fatal(err)
		}
		tx.MustExec("INSERT INTO person (first_name, last_name, email) VALUES (?, ?, ?)", "Code", "Hex", "x00.x7f@gmail.com")
		tx.MustExec("UPDATE person SET email = ? WHERE first_name = ? AND last_name = ?", "a@b.com", "Code", "Hex")
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}

		var author Person
		if err := db.Get(&author, "SELECT * FROM person WHERE email = ?", "a@b.com"); err != sql.ErrNoRows {
			t.Fatal(errors.Wrapf(err, "rollback test is failed, %s", db.activeTx.String()))
		}
	})
}

func TestNestedCommit(t *testing.T) {
	nested := func(db *DB) {
		tx, err := db.BeginTxm()
		if err != nil {
			t.Fatal(err)
		}

	}
	RunWithSchema(defaultSchema, t, func(db *DB, t *testing.T) {
		tx, err := db.BeginTxm()
		if err != nil {
			t.Fatal(err)
		}
		tx.MustExec("INSERT INTO person (first_name, last_name, email) VALUES (?, ?, ?)", "Code", "Hex", "x00.x7f@gmail.com")
		tx.MustExec("UPDATE person SET email = ? WHERE first_name = ? AND last_name = ?", "a@b.com", "Code", "Hex")
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}

		var author Person
		if err := db.Get(&author, "SELECT * FROM person WHERE email = ?", "a@b.com"); err != sql.ErrNoRows {
			t.Fatal(errors.Wrapf(err, "rollback test is failed, %s", db.activeTx.String()))
		}
	})
}
