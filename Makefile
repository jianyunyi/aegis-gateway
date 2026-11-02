# AEGIS 常用命令入口（Windows 无 make 时，见 README 的直接命令对照）
.PHONY: dev build build-linux test vet tidy fmt ensure-compose-env up down logs ps migrate

dev: ensure-compose-env ## 本地运行网关（需先启动 mysql/redis）
	cd backend && if [ -f ../.env ]; then set -a; . ../.env; set +a; fi; \
	APP_ENV=dev AUTO_MIGRATE=true MYSQL_DSN_FILE=../.secrets/mysql_dsn_host \
	REDIS_ADDR=127.0.0.1:6380 REDIS_PASSWORD_FILE=../.secrets/redis_password \
	JWT_SECRET_FILE=../.secrets/jwt_secret \
	BOOTSTRAP_ADMIN_PASSWORD_FILE=../.secrets/bootstrap_admin_password go run ./cmd/gateway

build:      ## 编译网关二进制（Windows）
	cd backend && go build -o bin/gateway.exe ./cmd/gateway

build-linux: ## 交叉编译 Linux 二进制（docker 离线构建用）
	cd backend && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/gateway-linux ./cmd/gateway
	cd backend && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/mockupstream-linux ./cmd/mockupstream

test:       ## 运行后端全部单测（含竞态检测）
	cd backend && go test ./... -race -count=1

vet:        ## 静态检查
	cd backend && go vet ./...

tidy:       ## 整理依赖
	cd backend && go mod tidy

fmt:        ## 代码格式检查
	cd backend && gofmt -l .

ensure-compose-env: ## 初始化本地 Compose 密钥（不会覆盖已有 .env 或 .secrets）
	@if [ ! -f .env ] && [ ! -d .secrets ]; then ./scripts/init-compose-env.sh; \
	elif [ ! -f .env ] || [ ! -d .secrets ]; then \
	  echo 'Expected both .env and .secrets; inspect the incomplete local setup before continuing.' >&2; exit 1; \
	fi

up: ensure-compose-env ## 一键启动全栈（构建镜像并后台运行）
	docker compose --env-file .env -f deploy/docker-compose.yml up -d --build

down:       ## 停止全栈
	docker compose --env-file .env -f deploy/docker-compose.yml down

logs:       ## 跟随日志
	docker compose --env-file .env -f deploy/docker-compose.yml logs -f

ps:         ## 查看服务状态
	docker compose --env-file .env -f deploy/docker-compose.yml ps

migrate: ensure-compose-env ## 仅执行数据库迁移
	cd backend && if [ -f ../.env ]; then set -a; . ../.env; set +a; fi; \
	APP_ENV=prod AUTO_MIGRATE=false MYSQL_DSN_FILE=../.secrets/mysql_dsn_host go run ./cmd/gateway -migrate-only
