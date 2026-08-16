# AEGIS 常用命令入口（Windows 无 make 时，见 README 的直接命令对照）
.PHONY: dev build test vet tidy fmt up down logs ps

dev:        ## 本地运行网关（需先启动 mysql/redis）
	cd backend && go run ./cmd/gateway

build:      ## 编译网关二进制
	cd backend && go build -o bin/gateway ./cmd/gateway

test:       ## 运行后端全部单测（含竞态检测）
	cd backend && go test ./... -race -count=1

vet:        ## 静态检查
	cd backend && go vet ./...

tidy:       ## 整理依赖
	cd backend && go mod tidy

fmt:        ## 代码格式检查
	cd backend && gofmt -l .

up:         ## 一键启动全栈（构建镜像并后台运行）
	docker compose -f deploy/docker-compose.yml up -d --build

down:       ## 停止全栈
	docker compose -f deploy/docker-compose.yml down

logs:       ## 跟随日志
	docker compose -f deploy/docker-compose.yml logs -f

ps:         ## 查看服务状态
	docker compose -f deploy/docker-compose.yml ps

migrate:    ## 仅执行数据库迁移
	cd backend && go run ./cmd/gateway -migrate-only
