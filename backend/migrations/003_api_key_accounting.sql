-- 补齐旧 SQL 基线遗漏的 API Key 账务字段；新库已在 001 中创建这些列。
SET @aegis_schema = DATABASE();

SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = @aegis_schema AND table_name = 'api_keys' AND column_name = 'used_tokens') = 0,
  'ALTER TABLE api_keys ADD COLUMN used_tokens BIGINT NOT NULL DEFAULT 0',
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;

SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = @aegis_schema AND table_name = 'api_keys' AND column_name = 'default_model') = 0,
  CONCAT('ALTER TABLE api_keys ADD COLUMN default_model VARCHAR(64) NOT NULL DEFAULT ', QUOTE('')),
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;

SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = @aegis_schema AND table_name = 'api_keys' AND column_name = 'budget_monthly') = 0,
  'ALTER TABLE api_keys ADD COLUMN budget_monthly DECIMAL(12,4) NOT NULL DEFAULT 0',
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;
