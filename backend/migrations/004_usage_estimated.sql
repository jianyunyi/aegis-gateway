-- Mark usage estimated from request/response data when an upstream omits usage.
SET @aegis_schema = DATABASE();

SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = @aegis_schema AND table_name = 'usage_logs' AND column_name = 'usage_estimated') = 0,
  'ALTER TABLE usage_logs ADD COLUMN usage_estimated TINYINT(1) NOT NULL DEFAULT 0',
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;
