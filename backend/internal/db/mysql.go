package db

import (
	"context"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// DB wraps a sqlx.DB for MariaDB/MySQL access.
type DB struct {
	Conn *sqlx.DB
}

// New connects to MariaDB/MySQL using the given DSN.
// DSN format: user:password@tcp(host:port)/dbname?parseTime=true&charset=utf8mb4
func New(ctx context.Context, dsn string) (*DB, error) {
	// Enforce safe defaults on the DSN
	safeDSN := dsn +
		"&parseTime=true" +
		"&charset=utf8mb4" +
		"&collation=utf8mb4_unicode_ci" +
		"&multiStatements=false" + // prevent SQL injection via stacked statements
		"&sql_mode=STRICT_ALL_TABLES" // treat truncation/type errors as errors, not warnings

	conn, err := sqlx.ConnectContext(ctx, "mysql", safeDSN)
	if err != nil {
		return nil, fmt.Errorf("connecting to mariadb: %w", err)
	}

	// Connection pool tuning
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(10)
	// Recycle connections before MariaDB's wait_timeout (default 8h) closes them
	conn.SetConnMaxLifetime(4 * time.Hour)
	// Drop idle connections that haven't been used in a while
	conn.SetConnMaxIdleTime(10 * time.Minute)

	// Verify the connection is actually alive
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("pinging mariadb: %w", err)
	}

	// Enforce InnoDB and strict mode for this session
	if _, err := conn.ExecContext(ctx, `SET SESSION sql_mode = 'STRICT_ALL_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'`); err != nil {
		conn.Close()
		return nil, fmt.Errorf("setting session sql_mode: %w", err)
	}

	return &DB{Conn: conn}, nil
}

// Close closes the database connection pool.
func (d *DB) Close() {
	d.Conn.Close()
}
