APP_NAME := worklog
VERSION := $(shell cat VERSION | tr -d '\n')
BUILD_TIME := $(shell date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X smallgo/server/version.Version=$(VERSION) -X smallgo/server/version.BuildTime=$(BUILD_TIME) -X smallgo/server/version.GitCommit=$(GIT_COMMIT) -X smallgo/server/version.AppName=$(APP_NAME)

.PHONY: help dev start build build-frontend build-backend build-linux build-docker build-docker-multi build-all fnpack clean

help:
	@echo "用法: make [目标]"
	@echo ""
	@echo "目标:"
	@echo "  help            显示本帮助"
	@echo "  dev             构建到 dev/ 目录并启动（模拟生产环境）"
	@echo "  start           直接启动服务（go run）"
	@echo "  build           构建前端 + 后端"
	@echo "  build-frontend  仅构建前端"
	@echo "  build-backend   仅构建后端"
	@echo "  build-linux     构建 linux/amd64 版本"
	@echo "  build-docker    构建 Docker 镜像"
	@echo "  build-docker-multi 构建 docker 多平台合并镜像（OCI 归档，PUSH=1 推 Docker Hub）"
	@echo "  build-all       构建所有平台"
	@echo "  fnpack          打飞牛 fnOS 安装包"
	@echo "  clean           清理构建产物"

dev:
	@echo "[1/4] 清理旧产物（保留 dev/data 数据）..."
	@rm -rf dev/static dev/$(APP_NAME)
	@mkdir -p dev/static/dist dev/data
	@echo "[2/4] 构建前端（详细输出见 dev/build.log）..."
	@cd web && npm ci > ../dev/build.log 2>&1 \
		&& npm run build >> ../dev/build.log 2>&1 \
		|| { echo "前端构建失败，dev/build.log 末尾内容："; tail -n 40 ../dev/build.log; exit 1; }
	@cp -r web/dist/* dev/static/dist/
	@echo "[3/4] 构建后端..."
	@cd server && go build -ldflags "$(LDFLAGS)" -o ../dev/$(APP_NAME) . >> ../dev/build.log 2>&1 \
		|| { echo "后端构建失败，dev/build.log 末尾内容："; tail -n 40 ../dev/build.log; exit 1; }
	@echo "[4/4] 启动服务..."
	@cd dev && ./$(APP_NAME) -data-dir=./data -web-dir=./static/dist

start:
	cd server && go run -ldflags "$(LDFLAGS)" .

build: build-frontend build-backend

build-frontend:
	cd web && npm ci && npm run build
	rm -rf server/static/dist
	mkdir -p server/static
	cp -r web/dist server/static/dist

build-backend:
	mkdir -p build
	cd server && go build -ldflags "$(LDFLAGS)" -o ../build/$(APP_NAME) .

build-linux:
	docker run --rm \
		-v "$(CURDIR)/server:/src" \
		-v "go-build-cache:/root/.cache/go-build" \
		-v "go-mod-cache:/go/pkg/mod" \
		-w /src \
		--platform linux/amd64 \
		-e "LDFLAGS=$(LDFLAGS)" \
		golang:1.26-alpine \
		sh -c 'apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go build -ldflags "$$LDFLAGS -extldflags -static" -o worklog-linux-amd64 .'

build-docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg BUILD_TIME=$(BUILD_TIME) --build-arg GIT_COMMIT=$(GIT_COMMIT) -t techfunways/$(APP_NAME):$(VERSION) .

build-docker-multi:
	bash scripts/build-docker-multi.sh

build-all:
	bash scripts/build-all.sh

fnpack:
	bash scripts/build-fnpack.sh

clean:
	rm -f server/$(APP_NAME) server/$(APP_NAME)-*
	rm -rf server/static/dist
	rm -rf web/dist
	rm -rf build/
	rm -rf dev/static dev/$(APP_NAME)
