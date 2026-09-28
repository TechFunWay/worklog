package audit

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"
	"smallgo/server/sysconfig"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB(db) })
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := sysconfig.InitDefaultConfigs(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMutationLoggerRecordsSuccessfulWritesOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAuditDB(t)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", uint(7))
		c.Set("username", "alice")
	})
	r.Use(MutationLogger(db))
	r.PUT("/api/profile", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.POST("/api/explicit", func(c *gin.Context) {
		Log(db, c, "profile_update", "user", 7, "updated profile")
		c.Status(http.StatusNoContent)
	})

	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/api/profile"},
		{http.MethodPost, "/api/explicit"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(request.method, request.path, nil))
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s %s: status %d", request.method, request.path, w.Code)
		}
	}

	var rows []database.AuditLog
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Action != "put_api_profile" || rows[1].Action != "profile_update" {
		t.Fatalf("unexpected audit rows: %#v", rows)
	}
}

func TestCleanupRespectsRetentionDays(t *testing.T) {
	db := setupAuditDB(t)
	if err := sysconfig.UpdateConfig(db, "log_retention_days", "7", 0); err != nil {
		t.Fatal(err)
	}
	old := database.AuditLog{Action: "old", CreatedAt: time.Now().AddDate(0, 0, -8)}
	recent := database.AuditLog{Action: "recent", CreatedAt: time.Now().AddDate(0, 0, -6)}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&recent).Error; err != nil {
		t.Fatal(err)
	}
	cleanup(db)
	var actions []string
	if err := db.Model(&database.AuditLog{}).Order("id").Pluck("action", &actions).Error; err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0] != "recent" {
		t.Fatalf("retention cleanup kept %v, want [recent]", actions)
	}
}
