package migration

import (
	"context"
	"database/sql"
	"fmt"
)

// withForeignKeysDisabled 把外键 PRAGMA、事务、校验和恢复固定在同一条 SQLite 连接上。
// SQLite 在事务内切换 foreign_keys 是 no-op，因此必须先获取独占连接并在 BEGIN 前关闭。
func withForeignKeysDisabled(ctx context.Context, db *sql.DB, migrate func(*sql.Tx) error) (resultErr error) {
	if db == nil {
		return fmt.Errorf("migration database is nil")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Close()

	var original int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&original); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	defer func() {
		target := 0
		if original != 0 {
			target = 1
		}
		if _, err := conn.ExecContext(context.Background(), fmt.Sprintf("PRAGMA foreign_keys=%d", target)); err != nil {
			if resultErr == nil {
				resultErr = fmt.Errorf("restore foreign_keys pragma: %w", err)
			}
			return
		}
		var restored int
		if err := conn.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&restored); err != nil {
			if resultErr == nil {
				resultErr = fmt.Errorf("verify restored foreign_keys pragma: %w", err)
			}
			return
		}
		if restored != target && resultErr == nil {
			resultErr = fmt.Errorf("foreign_keys pragma restored to %d, want %d", restored, target)
		}
	}()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("disable foreign_keys pragma: %w", err)
	}
	var disabled int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&disabled); err != nil {
		return fmt.Errorf("verify disabled foreign_keys pragma: %w", err)
	}
	if disabled != 0 {
		return fmt.Errorf("foreign_keys pragma is still enabled before migration")
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback()
	if err := migrate(tx); err != nil {
		return err
	}
	if err := verifyForeignKeysTx(ctx, tx); err != nil {
		return err
	}
	if err := verifyIntegrityTx(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}
	return nil
}

func verifyForeignKeysTx(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("run foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID sql.NullInt64
		var parent string
		var constraint int
		if err := rows.Scan(&table, &rowID, &parent, &constraint); err != nil {
			return fmt.Errorf("scan foreign_key_check: %w", err)
		}
		return fmt.Errorf("foreign_key_check failed: table=%s rowid=%d parent=%s constraint=%d", table, rowID.Int64, parent, constraint)
	}
	return rows.Err()
}

func verifyIntegrityTx(ctx context.Context, tx *sql.Tx) error {
	var result string
	if err := tx.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("run integrity_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check returned %q", result)
	}
	return nil
}
