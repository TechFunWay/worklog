#!/usr/bin/env bash
# new-app.sh — SmallGo 新应用模块脚手架
#
# 用法:
#   ./scripts/new-app.sh <name> "<显示名>"
#
#   name      模块 ID：小写字母开头，仅含小写字母/数字/下划线（如 todo、note_book）
#   显示名    可选，侧边栏与页面标题（默认取 name）
#
# 生成内容:
#   server/<name>/<name>.go       后端 app 模块（GORM 模型 + 分页列表/创建/删除示例路由）
#   web/src/api/<name>.ts         前端 API client
#   web/src/views/<Name>View.vue  前端页面（PageHeader + 列表 + 新建 + 删除）
#
# 生成后需要手工完成的三步（脚本结尾会再次提示）:
#   1. server/main.go            添加 _ "smallgo/server/<name>" 空导入
#   2. web/src/router/index.ts   在 /admin children 中注册路由
#   3. web/src/layouts/MainLayout.vue 的 mainNav/adminNav 中加入导航项
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ $# -lt 1 ]]; then
  echo "用法: $0 <name> [显示名]" >&2
  exit 1
fi

NAME="$1"
if [[ ! "$NAME" =~ ^[a-z][a-z0-9_]*$ ]]; then
  echo "错误: 模块名必须是 小写字母开头 + 小写字母/数字/下划线（例如 todo），收到: $NAME" >&2
  exit 1
fi

DISPLAY="${2:-$NAME}"
# PascalCase：按下划线/连字符切分后各段首字母大写（awk，兼容 BSD/GNU sed）
PASCAL=$(echo "$NAME" | awk -F '[_-]+' '{ out=""; for (i = 1; i <= NF; i++) { seg = $i; out = out toupper(substr(seg, 1, 1)) substr(seg, 2) } print out }')

BACKEND_DIR="$REPO_ROOT/server/$NAME"
API_FILE="$REPO_ROOT/web/src/api/$NAME.ts"
VIEW_FILE="$REPO_ROOT/web/src/views/${PASCAL}View.vue"

for target in "$BACKEND_DIR" "$API_FILE" "$VIEW_FILE"; do
  if [[ -e "$target" ]]; then
    echo "错误: $target 已存在，拒绝覆盖" >&2
    exit 1
  fi
done

mkdir -p "$BACKEND_DIR"

# ---------------------------------------------------------------------------
# 后端 app 模块：模型 + init() 注册 + 分页列表/创建/删除
# ---------------------------------------------------------------------------
cat > "$BACKEND_DIR/$NAME.go" <<EOF
package $NAME

import (
	"net/http"
	"strconv"
	"time"

	"smallgo/server/apps"
	"smallgo/server/database"
	"smallgo/server/response"
	"smallgo/server/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ${PASCAL}Item 是本应用的业务模型，经 RegisterModels 并入启动时的
// AutoMigrate（与框架核心表同一批次建表）。
type ${PASCAL}Item struct {
	ID        uint   \`gorm:"primarykey"\`
	UserID    uint   \`gorm:"index;not null"\`
	Title     string \`gorm:"not null"\`
	Done      bool   \`gorm:"default:false"\`
	CreatedAt time.Time \`gorm:"index"\`
}

func init() {
	database.RegisterModels(&${PASCAL}Item{})

	// 如需一次性数据迁移（记录进 upgrade_records），按需启用：
	//
	// database.Upgrades = append(database.Upgrades, database.Upgrade{
	// 	Version: "0.0.1",
	// 	Name:    "${NAME}_seed",
	// 	Upgrade: func(db *gorm.DB) error { return nil },
	// })

	apps.Register(apps.App{
		Name:        "$NAME",
		DisplayName: "$DISPLAY",
		Icon:        "app",
		NavPosition: 100,
		Setup:       setupRoutes,
	})
}

// setupRoutes 只挂业务路由。认证已由框架分组完成：
// Setup 拿到的 api 组在 authGroup 下（登录可用）；
// 需要管理员的路由改用 SetupAdmin（adminGroup）。
func setupRoutes(api *gin.RouterGroup, db *gorm.DB) {
	api.GET("/$NAME", handleList(db))
	api.POST("/$NAME", handleCreate(db))
	api.DELETE("/$NAME/:id", handleDelete(db))
}

func currentUserID(c *gin.Context) uint {
	return c.GetUint("userID")
}

// handleList 分页返回当前用户的条目。分页参数与错误码遵循框架约定
// （utils.NormalizePage + response.SuccessPage）。
func handleList(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := utils.Atoi(c.Query("page"), 1)
		pageSize := utils.Atoi(c.Query("pageSize"), 20)

		query := db.Model(&${PASCAL}Item{}).Where("user_id = ?", currentUserID(c))

		var total int64
		if err := query.Count(&total).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}

		items := make([]${PASCAL}Item, 0)
		if err := query.Order("id DESC").
			Offset(utils.Offset(page, pageSize)).
			Limit(pageSize).
			Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}

		response.SuccessPage(c, items, total, page, pageSize)
	}
}

func handleCreate(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Title string \`json:"title" binding:"required"\`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "标题不能为空")
			return
		}

		item := ${PASCAL}Item{UserID: currentUserID(c), Title: req.Title, CreatedAt: time.Now()}
		if err := db.Create(&item).Error; err != nil {
			response.ErrorInternal(c, "创建失败")
			return
		}

		response.Success(c, item)
	}
}

func handleDelete(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil || id <= 0 {
			response.ErrorBadRequest(c, "无效的 ID")
			return
		}

		// 带上 user_id 条件：用户只能删除自己的数据。
		result := db.Where("id = ? AND user_id = ?", id, currentUserID(c)).
			Delete(&${PASCAL}Item{})
		if result.Error != nil {
			response.ErrorInternal(c, "删除失败")
			return
		}
		if result.RowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "条目不存在"})
			return
		}

		response.Success(c, nil)
	}
}
EOF

# ---------------------------------------------------------------------------
# 前端 API client
# ---------------------------------------------------------------------------
cat > "$API_FILE" <<EOF
import request from './request'

export interface ${PASCAL}Item {
  id: number
  title: string
  done: boolean
  created_at: string
}

export function get${PASCAL}Items(page = 1, pageSize = 20) {
  return request.get('/api/$NAME', { params: { page, pageSize } })
}

export function create${PASCAL}Item(title: string) {
  return request.post('/api/$NAME', { title })
}

export function delete${PASCAL}Item(id: number) {
  return request.delete(\`/api/$NAME/\${id}\`)
}
EOF

# ---------------------------------------------------------------------------
# 前端视图
# ---------------------------------------------------------------------------
cat > "$VIEW_FILE" <<EOF
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import { create${PASCAL}Item, delete${PASCAL}Item, get${PASCAL}Items, type ${PASCAL}Item } from '../api/$NAME'

const items = ref<${PASCAL}Item[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const newTitle = ref('')

async function load() {
  loading.value = true
  try {
    const res = await get${PASCAL}Items(page.value, pageSize)
    if (res.data?.code === 0) {
      items.value = res.data.data.items || []
      total.value = res.data.data.total || 0
    }
  } finally {
    loading.value = false
  }
}

async function create() {
  const title = newTitle.value.trim()
  if (!title) return
  await create${PASCAL}Item(title)
  newTitle.value = ''
  await load()
}

async function remove(id: number) {
  if (!window.confirm('确认删除该条目？')) return
  await delete${PASCAL}Item(id)
  await load()
}

onMounted(load)
</script>

<template>
  <div class="page-container">
    <PageHeader title="$DISPLAY" description="$DISPLAY — 由 scripts/new-app.sh 生成的示例页面，按需改造。" />

    <div class="surface rounded-2xl p-5 sm:p-6 space-y-4">
      <form class="flex gap-3" @submit.prevent="create">
        <input
          v-model="newTitle"
          class="input-field flex-1"
          placeholder="输入标题后回车创建…"
        />
        <button type="submit" class="btn-brand !px-5 whitespace-nowrap">新建</button>
      </form>

      <div v-if="loading" class="text-sm text-muted-foreground py-8 text-center">加载中…</div>
      <div v-else-if="items.length === 0" class="text-sm text-muted-foreground py-8 text-center">暂无数据</div>
      <ul v-else class="divide-y divide-border">
        <li v-for="item in items" :key="item.id" class="flex items-center justify-between py-3">
          <span class="text-sm text-foreground">{{ item.title }}</span>
          <button class="text-sm text-red-500 hover:underline" @click="remove(item.id)">删除</button>
        </li>
      </ul>

      <div v-if="total > pageSize" class="flex items-center justify-between text-sm text-muted-foreground">
        <span>共 {{ total }} 条</span>
        <div class="flex gap-2">
          <button class="btn-secondary !px-3 !py-1" :disabled="page <= 1" @click="page--; load()">上一页</button>
          <span class="px-2 py-1">第 {{ page }} 页</span>
          <button class="btn-secondary !px-3 !py-1" :disabled="page * pageSize >= total" @click="page++; load()">下一页</button>
        </div>
      </div>
    </div>
  </div>
</template>
EOF

echo "已生成:"
echo "  server/$NAME/$NAME.go"
echo "  web/src/api/$NAME.ts"
echo "  web/src/views/${PASCAL}View.vue"
echo
echo "接下来请手工完成（三步）:"
echo "  1. server/main.go: 添加空导入  _ \"smallgo/server/$NAME\""
echo "  2. web/src/router/index.ts: /admin children 中添加"
echo "     { path: '$NAME', name: '${PASCAL}', component: () => import('../views/${PASCAL}View.vue') },"
echo "  3. web/src/layouts/MainLayout.vue: mainNav 或 adminNav 中加入 { to: '/admin/$NAME', label: '$DISPLAY', icon: <svg…> }"
echo
echo "验证:"
echo "  cd server && go build ./... && go test ./..."
echo "  cd web && npx vue-tsc --noEmit"
