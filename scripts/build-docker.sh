#!/bin/bash
set -e

APP_NAME="worklog"
# 镜像名与 Makefile / 发行 compose 保持一致：techfunways/worklog:<带v版本>
IMAGE_NAME="techfunways/${APP_NAME}"
VERSION=$(cat VERSION | tr -d '\n')
BUILD_TIME=$(date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

echo "Building Docker image..."
docker build \
  --build-arg VERSION=${VERSION} \
  --build-arg BUILD_TIME=${BUILD_TIME} \
  --build-arg GIT_COMMIT=${GIT_COMMIT} \
  -t ${IMAGE_NAME}:${VERSION} \
  -t ${IMAGE_NAME}:latest \
  .

echo "Docker image built: ${IMAGE_NAME}:${VERSION}"
