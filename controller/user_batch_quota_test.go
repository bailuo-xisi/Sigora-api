package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type batchQuotaAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Count int `json:"count"`
	} `json:"data"`
}

func setupUserBatchQuotaControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL

	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func performBatchQuotaRequest(t *testing.T, role int, request BatchAdjustUserQuotaRequest) batchQuotaAPIResponse {
	t.Helper()

	body, err := common.Marshal(request)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/batch/quota", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("id", 1)
	context.Set("username", "operator")
	context.Set("role", role)

	BatchAdjustUserQuota(context)

	var response batchQuotaAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func seedBatchQuotaUsers(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "root", Role: common.RoleRootUser, Quota: 1000, AffCode: "root-code"},
		{Id: 2, Username: "user-2", Role: common.RoleCommonUser, Quota: 100, AffCode: "user-2-code"},
		{Id: 3, Username: "user-3", Role: common.RoleCommonUser, Quota: 200, AffCode: "user-3-code"},
		{Id: 4, Username: "admin-4", Role: common.RoleAdminUser, Quota: 300, AffCode: "admin-4-code"},
	}).Error)
}

func loadBatchQuotaValues(t *testing.T, db *gorm.DB, ids ...int) map[int]int {
	t.Helper()
	var users []model.User
	require.NoError(t, db.Select("id", "quota").Where("id IN ?", ids).Find(&users).Error)
	values := make(map[int]int, len(users))
	for _, user := range users {
		values[user.Id] = user.Quota
	}
	return values
}

func TestBatchAdjustUserQuotaModes(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		value    int
		expected map[int]int
	}{
		{name: "add", mode: "add", value: 25, expected: map[int]int{2: 125, 3: 225}},
		{name: "subtract", mode: "subtract", value: 25, expected: map[int]int{2: 75, 3: 175}},
		{name: "override", mode: "override", value: 25, expected: map[int]int{2: 25, 3: 25}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupUserBatchQuotaControllerTestDB(t)
			seedBatchQuotaUsers(t, db)

			response := performBatchQuotaRequest(t, common.RoleRootUser, BatchAdjustUserQuotaRequest{
				Ids:   []int{2, 2, 3},
				Mode:  test.mode,
				Value: test.value,
			})

			require.True(t, response.Success, response.Message)
			require.Equal(t, 2, response.Data.Count)
			require.Equal(t, test.expected, loadBatchQuotaValues(t, db, 2, 3))
		})
	}
}

func TestBatchAdjustUserQuotaRejectsMissingUserWithoutPartialUpdate(t *testing.T) {
	db := setupUserBatchQuotaControllerTestDB(t)
	seedBatchQuotaUsers(t, db)

	response := performBatchQuotaRequest(t, common.RoleRootUser, BatchAdjustUserQuotaRequest{
		Ids:   []int{2, 999},
		Mode:  "add",
		Value: 50,
	})

	require.False(t, response.Success)
	require.Equal(t, map[int]int{2: 100}, loadBatchQuotaValues(t, db, 2))
}

func TestBatchAdjustUserQuotaRejectsUnmanageableUserWithoutPartialUpdate(t *testing.T) {
	db := setupUserBatchQuotaControllerTestDB(t)
	seedBatchQuotaUsers(t, db)

	response := performBatchQuotaRequest(t, common.RoleAdminUser, BatchAdjustUserQuotaRequest{
		Ids:   []int{2, 4},
		Mode:  "add",
		Value: 50,
	})

	require.False(t, response.Success)
	require.Equal(t, map[int]int{2: 100, 4: 300}, loadBatchQuotaValues(t, db, 2, 4))
}
