#!/bin/bash
# 构建 docker 多平台合并镜像（linux/amd64 + linux/arm64），默认导出本地 OCI 归档，
# 归档随 release/<版本>/ 发行目录分发，目标机器 docker load -i 即可用。
#
# 环境变量（默认都不开，保持"未经用户确认不推送远端"的约定）：
#   PUSH=1      构建后把同一份合并 manifest 推到 Docker Hub（techfunways/worklog），
#               tag 为 :<版本> 与 :latest，需先 docker login（账号须能写该命名空间）
#   SKIP_OCI=1  跳过本地 OCI 归档（只推送时用，避免重打已随发行目录分发的归档）
# 也支持 `--push` 作为 PUSH=1 的等价参数。
#
# 依赖 ./scripts/build-all.sh 先跑出两个 linux 平台压缩包（Dockerfile 直接用
# 里面编译好的静态二进制与前端产物，不再在 buildx 里重编译，速度快且与
# tar.gz 里的产物完全一致）。
set -e

PROJECT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"
cd "$PROJECT_DIR"

PUSH="${PUSH:-0}"
SKIP_OCI="${SKIP_OCI:-0}"
for arg in "$@"; do
    [ "$arg" = "--push" ] && PUSH=1
done

VERSION=$(cat VERSION | tr -d '\n')
[ -z "$VERSION" ] && echo "❌ 无法获取版本号" && exit 1

APP_NAME="techfunway-worklog"
IMAGE_NAME="techfunways/worklog"
RELEASE_DIR="${PROJECT_DIR}/release/${VERSION}"
BUILDER_NAME="${BUILDER_NAME:-worklog-multiarch}"
OCI_FILE="${RELEASE_DIR}/${APP_NAME}-${VERSION}-multiarch.oci.tar"

# build-all.sh 压缩后即清理中间目录；Docker 构建需要 linux 平台目录，
# 缺失时从对应的 tar.gz 解包，用完在脚本末尾清理
DOCKER_TMP_DIRS=()
for arch in amd64 arm64; do
    DIR_NAME="${APP_NAME}-${VERSION}-linux-${arch}"
    if [ ! -d "${RELEASE_DIR}/${DIR_NAME}" ]; then
        TAR_FILE="${RELEASE_DIR}/${DIR_NAME}.tar.gz"
        [ ! -f "${TAR_FILE}" ] && echo "❌ 缺少 ${TAR_FILE}，请先运行 ./scripts/build-all.sh" && exit 1
        tar -xzf "${TAR_FILE}" -C "${RELEASE_DIR}"
        DOCKER_TMP_DIRS+=("${RELEASE_DIR}/${DIR_NAME}")
    fi
done

echo "============================================"
echo "  Docker 多平台镜像 ${VERSION}（OCI=${SKIP_OCI} 推送=${PUSH}）"
echo "============================================"

# 删除 .DS_Store
find "${RELEASE_DIR}" -name ".DS_Store" -delete 2>/dev/null || true

# 切换到 default context
docker context use default 2>/dev/null || true

# 合并 manifest 的 OCI 归档必须用 docker-container 驱动的 builder，
# 默认驱动不推 registry 导不出多平台合并结果
if ! docker buildx inspect "${BUILDER_NAME}" >/dev/null 2>&1; then
    echo ""
    echo "🔧 创建 buildx builder: ${BUILDER_NAME}"
    docker buildx create --name "${BUILDER_NAME}" --driver docker-container --bootstrap >/dev/null
fi
docker buildx use "${BUILDER_NAME}"

echo ""
echo "🔨 构建多平台镜像 (linux/amd64 + linux/arm64)..."
echo ""

cd "${RELEASE_DIR}"

# 创建 Dockerfile：沿用仓库 Dockerfile 的运行时形态（alpine + 静态二进制 +
# 静态前端），二进制直接取 build-all.sh 的产物，避免 buildx 内重复编译
cat > Dockerfile << EOF
FROM alpine:3.20

ARG TARGETARCH
ARG VERSION

LABEL org.opencontainers.image.title="${APP_NAME}"
LABEL org.opencontainers.image.description="记工记账应用 — 按天/按时/计件记工，老板/组长/员工班组协作，借支与结算，工资条导出"
LABEL org.opencontainers.image.version="\${VERSION}"
LABEL org.opencontainers.image.source="https://github.com/TechFunWay/worklog"

# apk 源换阿里云镜像，避免官方 CDN 卡死（同 build-fnpack.sh 的处理）
RUN sed -i "s#https://dl-cdn.alpinelinux.org#https://mirrors.aliyun.com#" /etc/apk/repositories \\
    && apk add --no-cache ca-certificates tzdata \\
    && adduser -D -u 1000 appuser

WORKDIR /app

COPY ${APP_NAME}-${VERSION}-linux-\${TARGETARCH}/worklog ./worklog
COPY ${APP_NAME}-${VERSION}-linux-\${TARGETARCH}/static/dist ./static/dist

RUN mkdir -p /app/data && chown -R appuser:appuser /app
USER appuser

EXPOSE 8909
VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \\
  CMD wget -qO- http://localhost:8909/api/version || exit 1

ENTRYPOINT ["./worklog"]
CMD ["-data-dir", "/app/data", "-web-dir", "./static/dist"]
EOF

# 导出为本地 OCI 归档（不推送 registry），随发行目录分发
if [ "${SKIP_OCI}" != "1" ]; then
    docker buildx build \
        --builder "${BUILDER_NAME}" \
        --platform linux/amd64,linux/arm64 \
        --output "type=oci,dest=${OCI_FILE}" \
        --build-arg VERSION=${VERSION} \
        -t "${IMAGE_NAME}:${VERSION}" \
        -t "${IMAGE_NAME}:latest" \
        .
fi

# PUSH=1：把同一份多平台合并 manifest 推到 Docker Hub（需已 docker login）
if [ "${PUSH}" = "1" ]; then
    echo ""
    echo "🚀 推送多平台镜像到 Docker Hub: ${IMAGE_NAME}:${VERSION} / ${IMAGE_NAME}:latest"
    echo ""
    docker buildx build \
        --builder "${BUILDER_NAME}" \
        --platform linux/amd64,linux/arm64 \
        --push \
        --build-arg VERSION=${VERSION} \
        -t "${IMAGE_NAME}:${VERSION}" \
        -t "${IMAGE_NAME}:latest" \
        .
fi

rm -f Dockerfile
cd "${PROJECT_DIR}"

# 清理解包出来的临时平台目录，发行目录只留最终产物
for tmp_dir in "${DOCKER_TMP_DIRS[@]}"; do
    rm -rf "${tmp_dir}"
done

# 重新切回 default builder（避免影响后续 docker build）
docker buildx use default 2>/dev/null || true

echo ""
[ "${SKIP_OCI}" = "1" ] || echo "✅ 多平台 OCI 归档完成: ${OCI_FILE}"
[ "${PUSH}" = "1" ] && echo "✅ 已推送 Docker Hub: ${IMAGE_NAME}:${VERSION} / ${IMAGE_NAME}:latest（amd64 + arm64 合并 manifest）"
echo ""
[ "${SKIP_OCI}" = "1" ] || echo "  目标机器载入: docker load -i ${APP_NAME}-${VERSION}-multiarch.oci.tar"
echo "  本地镜像: ${IMAGE_NAME}:${VERSION} / ${IMAGE_NAME}:latest（含合并 manifest）"
echo ""
