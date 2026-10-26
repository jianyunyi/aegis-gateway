-- 评测可靠性补充迁移（适用于已使用旧版 001_init.sql 的数据库）。
-- 通过 information_schema 检查列，兼容旧版应用已由 AutoMigrate 创建 prompt_preview 的环境。

SET @aegis_schema = DATABASE();

SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = @aegis_schema AND table_name = 'usage_logs' AND column_name = 'prompt_preview') = 0,
  'ALTER TABLE usage_logs ADD COLUMN prompt_preview VARCHAR(500) NULL',
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;

SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = @aegis_schema AND table_name = 'usage_logs' AND column_name = 'prompt_for_eval') = 0,
  'ALTER TABLE usage_logs ADD COLUMN prompt_for_eval TEXT NULL',
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;

SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = @aegis_schema AND table_name = 'eval_samples' AND column_name = 'chat_messages') = 0,
  'ALTER TABLE eval_samples ADD COLUMN chat_messages TEXT NULL',
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;
