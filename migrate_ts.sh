#!/usr/bin/env bash
# 迁移现有数据库中的字符串时间戳为毫秒整数
# 用法: bash migrate_ts.sh /vol1/@appdata/wg-server/wg-server.db
set -e

DB="$1"
if [ -z "$DB" ]; then
  echo "Usage: $0 <path-to-wg-server.db>"
  exit 1
fi

BACKUP="${DB}.backup-$(date +%Y%m%d%H%M%S)"
cp "$DB" "$BACKUP"
echo "Backup saved: $BACKUP"

sqlite3 "$DB" << 'SQL'
-- backup: 将字符串 DATETIME 转为毫秒时间戳
-- 格式 "2006-01-02 15:04:05" 视为 Asia/Shanghai

-- users
ALTER TABLE users ADD COLUMN created_at_new INTEGER;
ALTER TABLE users ADD COLUMN updated_at_new INTEGER;
UPDATE users SET created_at_new = CAST(strftime('%s', created_at) AS INTEGER) * 1000;
UPDATE users SET updated_at_new = CAST(strftime('%s', updated_at) AS INTEGER) * 1000;

-- bandwidth_history
ALTER TABLE bandwidth_history ADD COLUMN ts_new INTEGER;
UPDATE bandwidth_history SET ts_new = CAST(strftime('%s', timestamp) AS INTEGER) * 1000;

-- system_log
ALTER TABLE system_log ADD COLUMN created_at_new INTEGER;
UPDATE system_log SET created_at_new = CAST(strftime('%s', created_at) AS INTEGER) * 1000;

-- 后续由新版代码自动用新列，旧列可待确认后手动删除
-- 删除旧列前先用新版运行确认数据正确
SQL

echo "Migration complete."
echo "New columns added: created_at_new, updated_at_new (users), ts_new (bandwidth_history), created_at_new (system_log)"
echo "The new code will use these columns. Old columns can be removed after verification."
