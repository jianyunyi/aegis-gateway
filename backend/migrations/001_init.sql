-- AEGIS 初始表结构（MySQL 8）
-- 说明：演示环境由 gateway 的 AutoMigrate 自动建表（backend/internal/model 为单一事实来源）；
-- 本文件作为生产环境 DDL 基线，二者保持一致。

CREATE DATABASE IF NOT EXISTS aegis DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE aegis;

CREATE TABLE IF NOT EXISTS users (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  username      VARCHAR(64)     NOT NULL,
  password_hash VARCHAR(255)    NOT NULL,
  role          VARCHAR(16)     NOT NULL DEFAULT 'admin',
  status        TINYINT         NOT NULL DEFAULT 1,
  created_at    DATETIME(3)     NOT NULL,
  updated_at    DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_username (username)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS api_keys (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name         VARCHAR(64)     NOT NULL,
  key_hash     CHAR(64)        NOT NULL COMMENT 'SHA-256，不存明文',
  key_prefix   VARCHAR(16)     NOT NULL COMMENT '展示用前缀 ak_xxxx',
  user_id      BIGINT UNSIGNED NOT NULL,
  status       TINYINT         NOT NULL DEFAULT 1 COMMENT '1 启用 0 禁用',
  quota_tokens BIGINT          NOT NULL DEFAULT 0 COMMENT '0=不限',
  rps_limit    INT             NOT NULL DEFAULT 10,
  burst        INT             NOT NULL DEFAULT 20,
  expires_at   DATETIME(3)     NULL,
  created_at   DATETIME(3)     NOT NULL,
  updated_at   DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_key_hash (key_hash),
  KEY idx_user (user_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS providers (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name        VARCHAR(32)     NOT NULL,
  base_url    VARCHAR(255)    NOT NULL,
  api_key_enc VARCHAR(512)    NOT NULL DEFAULT '' COMMENT 'AES-256 加密',
  enabled     TINYINT         NOT NULL DEFAULT 1,
  priority    INT             NOT NULL DEFAULT 0,
  created_at  DATETIME(3)     NOT NULL,
  updated_at  DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_name (name)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS models (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  provider_id    BIGINT UNSIGNED NOT NULL,
  name           VARCHAR(64)     NOT NULL COMMENT '上游模型名',
  display_name   VARCHAR(64)     NOT NULL DEFAULT '',
  tier           VARCHAR(16)     NOT NULL DEFAULT 'normal' COMMENT 'cheap/normal/strong',
  context_window INT             NOT NULL DEFAULT 8192,
  price_in       DECIMAL(10, 6)  NOT NULL DEFAULT 0 COMMENT '元/1K 输入',
  price_out      DECIMAL(10, 6)  NOT NULL DEFAULT 0 COMMENT '元/1K 输出',
  enabled        TINYINT         NOT NULL DEFAULT 1,
  created_at     DATETIME(3)     NOT NULL,
  updated_at     DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  KEY idx_provider (provider_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS usage_logs (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  request_id        CHAR(32)        NOT NULL,
  api_key_id        BIGINT UNSIGNED NOT NULL,
  user_id           BIGINT UNSIGNED NOT NULL DEFAULT 0,
  provider_id       BIGINT UNSIGNED NOT NULL DEFAULT 0,
  model_name        VARCHAR(64)     NOT NULL,
  kind              VARCHAR(16)     NOT NULL DEFAULT 'chat',
  prompt_tokens     INT             NOT NULL DEFAULT 0,
  completion_tokens INT             NOT NULL DEFAULT 0,
  total_tokens      INT             NOT NULL DEFAULT 0,
  cost              DECIMAL(12, 6)  NOT NULL DEFAULT 0,
  latency_ms        INT             NOT NULL DEFAULT 0,
  ttft_ms           INT             NULL,
  status            SMALLINT        NOT NULL DEFAULT 0 COMMENT '0 成功/4xx/5xx',
  error_code        VARCHAR(32)     NOT NULL DEFAULT '',
  cached            TINYINT         NOT NULL DEFAULT 0,
  routed_by         VARCHAR(16)     NOT NULL DEFAULT '' COMMENT 'manual/rule/heuristic/llm',
  upstream_model    VARCHAR(64)     NOT NULL DEFAULT '',
  created_at        DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_request_id (request_id),
  KEY idx_key_time (api_key_id, created_at),
  KEY idx_model_time (model_name, created_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS billing_daily (
  id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  date             DATE            NOT NULL,
  api_key_id       BIGINT UNSIGNED NOT NULL,
  request_count    INT             NOT NULL DEFAULT 0,
  prompt_tokens    INT             NOT NULL DEFAULT 0,
  completion_tokens INT            NOT NULL DEFAULT 0,
  total_tokens     INT             NOT NULL DEFAULT 0,
  cost             DECIMAL(12, 6)  NOT NULL DEFAULT 0,
  created_at       DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_date_key (date, api_key_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS eval_datasets (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name        VARCHAR(64)     NOT NULL,
  description VARCHAR(255)    NOT NULL DEFAULT '',
  status      TINYINT         NOT NULL DEFAULT 0 COMMENT '0 草稿 1 可用',
  created_at  DATETIME(3)     NOT NULL,
  updated_at  DATETIME(3)     NOT NULL,
  PRIMARY KEY (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS eval_samples (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  dataset_id BIGINT UNSIGNED NOT NULL,
  prompt     TEXT            NOT NULL,
  reference  TEXT            NULL,
  source     VARCHAR(16)     NOT NULL DEFAULT 'manual' COMMENT 'sampled/manual',
  label      TINYINT         NULL COMMENT '1 好 0 差 NULL 未标',
  created_at DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  KEY idx_dataset (dataset_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS eval_runs (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  dataset_id  BIGINT UNSIGNED NOT NULL,
  model_a     VARCHAR(64)     NOT NULL,
  model_b     VARCHAR(64)     NOT NULL,
  status      TINYINT         NOT NULL DEFAULT 0 COMMENT '0 运行 1 完成 2 失败',
  score_a     DECIMAL(5, 2)   NULL,
  score_b     DECIMAL(5, 2)   NULL,
  cost_a      DECIMAL(12, 6)  NULL,
  cost_b      DECIMAL(12, 6)  NULL,
  latency_a   INT             NULL,
  latency_b   INT             NULL,
  report      JSON            NULL,
  created_at  DATETIME(3)     NOT NULL,
  finished_at DATETIME(3)     NULL,
  PRIMARY KEY (id),
  KEY idx_dataset (dataset_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;
