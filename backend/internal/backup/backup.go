package backup

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Metadata struct {
	SchemaVersion int    `json:"schemaVersion"`
	Integrity     string `json:"integrity"`
	SizeBytes     int64  `json:"sizeBytes"`
}

func Create(ctx context.Context, db *sql.DB, destination string) error {
	if destination == "" {
		return fmt.Errorf("备份目标路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("备份目标已存在")
	}
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, destination); err != nil {
		return fmt.Errorf("create sqlite backup: %w", err)
	}
	if _, err := Validate(destination); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("validate created backup: %w", err)
	}
	return nil
}

func Validate(path string) (Metadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("open backup: %w", err)
	}
	header := make([]byte, 16)
	_, readErr := io.ReadFull(file, header)
	_ = file.Close()
	if readErr != nil || !bytes.Equal(header, []byte("SQLite format 3\x00")) {
		return Metadata{}, fmt.Errorf("文件不是有效的 SQLite 数据库")
	}
	info, err := os.Stat(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("stat backup: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return Metadata{}, fmt.Errorf("open backup read-only: %w", err)
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return Metadata{}, fmt.Errorf("数据库完整性校验失败: %s: %w", integrity, err)
	}
	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return Metadata{}, fmt.Errorf("读取迁移版本失败: %w", err)
	}
	if version < 1 || version > 1 {
		return Metadata{}, fmt.Errorf("不支持的数据库版本 %d", version)
	}
	return Metadata{SchemaVersion: version, Integrity: integrity, SizeBytes: info.Size()}, nil
}

func Restore(ctx context.Context, source, current string) error {
	if _, err := Validate(source); err != nil {
		return fmt.Errorf("恢复文件校验失败: %w", err)
	}
	if current == "" || source == current {
		return fmt.Errorf("恢复源与当前数据库路径无效")
	}
	backupPath := current + ".pre-restore-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if err := copyFile(current, backupPath); err != nil {
		return fmt.Errorf("恢复前备份当前数据库失败: %w", err)
	}
	temporary := current + ".restore-tmp"
	_ = os.Remove(temporary)
	if err := copyFile(source, temporary); err != nil {
		return fmt.Errorf("复制恢复文件失败: %w", err)
	}
	if _, err := Validate(temporary); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("临时恢复文件校验失败: %w", err)
	}
	select {
	case <-ctx.Done():
		_ = os.Remove(temporary)
		return ctx.Err()
	default:
	}
	if err := os.Rename(temporary, current); err != nil {
		return fmt.Errorf("替换当前数据库失败: %w", err)
	}
	_ = os.Remove(current + "-wal")
	_ = os.Remove(current + "-shm")
	return nil
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	removeOnError := true
	defer func() {
		_ = output.Close()
		if removeOnError {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	removeOnError = false
	return nil
}
