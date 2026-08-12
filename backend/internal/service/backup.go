package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/local/trade-discipline-desktop/backend/internal/backup"
)

func (s *Service) createAutomaticBackup(ctx context.Context, prefix string) error {
	directory := filepath.Join(filepath.Dir(s.store.Path()), "backups")
	destination := filepath.Join(directory, fmt.Sprintf("before-%s-%s.db", prefix, s.now().UTC().Format("20060102T150405.000000000Z")))
	if err := backup.Create(ctx, s.store.DB(), destination); err != nil {
		return fmt.Errorf("变更前自动备份失败: %w", err)
	}
	paths, err := filepath.Glob(filepath.Join(directory, "before-*.db"))
	if err != nil {
		return fmt.Errorf("读取自动备份列表失败: %w", err)
	}
	sort.Strings(paths)
	for len(paths) > 20 {
		if err := os.Remove(paths[0]); err != nil {
			return fmt.Errorf("清理旧自动备份失败: %w", err)
		}
		paths = paths[1:]
	}
	return nil
}

func (s *Service) ExportBackup(ctx context.Context, destination string) (backup.Metadata, error) {
	if strings.ToLower(filepath.Ext(destination)) != ".db" {
		return backup.Metadata{}, fmt.Errorf("备份文件必须使用 .db 扩展名")
	}
	if err := backup.Create(ctx, s.store.DB(), destination); err != nil {
		return backup.Metadata{}, err
	}
	return backup.Validate(destination)
}

func (s *Service) ValidateBackup(path string) (backup.Metadata, error) {
	return backup.Validate(path)
}
