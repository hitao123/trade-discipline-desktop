#!/bin/zsh
set -eu

project_dir="${0:A:h:h}"
app_path="$project_dir/release/mac-arm64/Plain Rule.app"
app_binary="$app_path/Contents/MacOS/Plain Rule"
smoke_dir="$(mktemp -d /private/tmp/shouzhuo-smoke.XXXXXX)"
database="$smoke_dir/discipline.db"

if [[ ! -x "$app_binary" ]]; then
  print -u2 "未找到打包应用，请先运行 pnpm build:mac"
  exit 1
fi

"$app_binary" --user-data-dir="$smoke_dir" &
app_pid=$!

for attempt in {1..60}; do
  if [[ -f "$database" ]]; then
    break
  fi
  sleep 0.5
done

if [[ ! -f "$database" ]]; then
  kill -TERM "$app_pid" 2>/dev/null || true
  print -u2 "首启未能在 30 秒内建立数据库：$smoke_dir"
  exit 1
fi

integrity="$(sqlite3 "$database" 'PRAGMA integrity_check;')"
account_count="$(sqlite3 "$database" 'SELECT count(*) FROM accounts;')"
rule_count="$(sqlite3 "$database" 'SELECT count(*) FROM rule_versions;')"
schema_version="$(sqlite3 "$database" 'SELECT max(version) FROM schema_migrations;')"
history_table_count="$(sqlite3 "$database" "SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('market_daily_bar_observations','market_metric_observations');")"
kill -TERM "$app_pid" 2>/dev/null || true
wait "$app_pid" 2>/dev/null || true

if [[ "$integrity" != "ok" || "$account_count" != "1" || "$rule_count" != "1" || "$schema_version" != "2" || "$history_table_count" != "2" ]]; then
  print -u2 "首启数据库校验失败：integrity=$integrity account=$account_count rule=$rule_count schema=$schema_version history_tables=$history_table_count"
  exit 1
fi

print "macOS 首启冒烟通过；临时数据：$smoke_dir"
