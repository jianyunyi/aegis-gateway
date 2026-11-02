-- New evaluation samples created after this migration receive an explicit
-- 30-day expiry, and manual samples also receive an auditable data-policy confirmation.
-- Existing rows intentionally remain NULL: operators must inventory and back
-- them up before deciding whether/how to clean them up.
SET @aegis_schema = DATABASE();
SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = @aegis_schema AND table_name = 'eval_samples' AND column_name = 'expires_at') = 0,
  'ALTER TABLE eval_samples ADD COLUMN expires_at DATETIME(3) NULL',
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;
SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = @aegis_schema AND table_name = 'eval_samples' AND column_name = 'manual_policy_confirmed_at') = 0,
  'ALTER TABLE eval_samples ADD COLUMN manual_policy_confirmed_at DATETIME(3) NULL',
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;
SET @aegis_ddl = IF(
  (SELECT COUNT(*) FROM information_schema.statistics
   WHERE table_schema = @aegis_schema AND table_name = 'eval_samples' AND index_name = 'idx_eval_samples_expires_at') = 0,
  'CREATE INDEX idx_eval_samples_expires_at ON eval_samples (expires_at)',
  'SELECT 1'
);
PREPARE aegis_stmt FROM @aegis_ddl;
EXECUTE aegis_stmt;
DEALLOCATE PREPARE aegis_stmt;
