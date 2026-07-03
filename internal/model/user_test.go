package model

import (
	"testing"
	"time"

	"api/common/idgen"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestUserPhysicalTableName 验证 2 的幂物理表数量路由规则稳定。
func TestUserPhysicalTableName(t *testing.T) {
	tests := []struct {
		name            string // name 表示测试场景名称。
		shardNo         int    // shardNo 表示逻辑分片号。
		routeShardCount int    // routeShardCount 表示物理路由分片数。
		want            string // want 表示期望结果。
	}{
		{name: "single", shardNo: 1023, routeShardCount: 1, want: "user"},
		{name: "two first", shardNo: 0, routeShardCount: 2, want: "user_0000"},
		{name: "two boundary", shardNo: 512, routeShardCount: 2, want: "user_0512"},
		{name: "four middle", shardNo: 700, routeShardCount: 4, want: "user_0512"},
		{name: "sixteen middle", shardNo: 345, routeShardCount: 16, want: "user_0320"},
		{name: "full last", shardNo: 1023, routeShardCount: 1024, want: "user_1023"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UserPhysicalTableName(tt.shardNo, tt.routeShardCount)
			if err != nil {
				t.Fatalf("UserPhysicalTableName() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UserPhysicalTableName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUserPhysicalTableNameRejectsInvalidRoute 验证路由数量只能按 2 的幂平滑拆分。
func TestUserPhysicalTableNameRejectsInvalidRoute(t *testing.T) {
	if _, err := UserPhysicalTableName(1, 3); err == nil {
		t.Fatal("期望非法物理表数量返回错误")
	}
	if _, err := UserPhysicalTableName(1024, 2); err == nil {
		t.Fatal("期望非法 shard_no 返回错误")
	}
}

// TestUserAccountTableNameRejectsMismatchedShardNo 验证账号索引不会接受错误分片号。
func TestUserAccountTableNameRejectsMismatchedShardNo(t *testing.T) {
	userID := int64(123456789)
	account := &UserAccount{
		UserID:          userID,
		ShardNo:         idgen.ShardNo(userID),
		RouteShardCount: 1024,
	}
	want, err := UserPhysicalTableName(account.ShardNo, account.RouteShardCount)
	if err != nil {
		t.Fatalf("UserPhysicalTableName() error = %v", err)
	}
	got, err := account.UserTableName()
	if err != nil {
		t.Fatalf("UserTableName() error = %v", err)
	}
	if got != want {
		t.Fatalf("UserTableName() = %q, want %q", got, want)
	}

	account.ShardNo = (account.ShardNo + 1) % userRouteShardMod
	if _, err := account.UserTableName(); err == nil {
		t.Fatal("期望账号索引 shard_no 与 user_id 不一致时返回错误")
	}
}

// TestSafeUserUpdatesRejectsImmutableFields 验证通用更新不会修改用户分片和唯一账号字段。
func TestSafeUserUpdatesRejectsImmutableFields(t *testing.T) {
	got := safeUserUpdates(map[string]any{
		"id":            int64(1),
		"shard_no":      12,
		"username":      "changed",
		"password_hash": "unsafe",
		"email":         "ok@example.com",
	}, false)
	for _, key := range []string{"id", "shard_no", "username", "password_hash"} {
		if _, ok := got[key]; ok {
			t.Fatalf("safeUserUpdates() should reject %s: %+v", key, got)
		}
	}
	if got["email"] != "ok@example.com" {
		t.Fatalf("safeUserUpdates() should keep email: %+v", got)
	}
}

// TestCreateUserWithAccountRoutesPhysicalTable 验证非默认物理表数量会真实写入并路由读取。
func TestCreateUserWithAccountRoutesPhysicalTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:user_route_test?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	userID, shardNo := userIDInShardRangeForTest(t, 512, 767)
	tableName, err := UserPhysicalTableName(shardNo, 4)
	if err != nil {
		t.Fatalf("UserPhysicalTableName() error = %v", err)
	}
	if tableName != "user_0512" {
		t.Fatalf("route table = %s, want user_0512 for shard=%d", tableName, shardNo)
	}
	if err = db.AutoMigrate(&userAccountSQLiteForTest{}); err != nil {
		t.Fatalf("AutoMigrate(user_account) error = %v", err)
	}
	if err = db.Table(tableName).AutoMigrate(&userSQLiteForTest{}); err != nil {
		t.Fatalf("AutoMigrate(%s) error = %v", tableName, err)
	}
	now := time.Now()
	user := &User{
		ID:           userID,
		ShardNo:      shardNo,
		Username:     "route_user",
		Nickname:     "route_user",
		PasswordHash: "hash",
		Status:       UserStatusEnabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err = CreateUserWithAccount(db, user, 4); err != nil {
		t.Fatalf("CreateUserWithAccount() error = %v", err)
	}
	var count int64
	if err = db.Table(tableName).Where("id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count routed user error = %v", err)
	}
	if count != 1 {
		t.Fatalf("routed table count = %d, want 1", count)
	}
	got, err := FindUserByUsername(db, "route_user")
	if err != nil {
		t.Fatalf("FindUserByUsername() error = %v", err)
	}
	if got == nil || got.ID != userID {
		t.Fatalf("FindUserByUsername() = %+v, want id=%d", got, userID)
	}
	account, err := FindUserAccountByUsername(db, "route_user")
	if err != nil {
		t.Fatalf("FindUserAccountByUsername() error = %v", err)
	}
	if account == nil || account.RouteShardCount != 4 || account.ShardNo != shardNo {
		t.Fatalf("account = %+v, want shard=%d route=4", account, shardNo)
	}
}

// userIDInShardRangeForTest 表示测试辅助逻辑。
func userIDInShardRangeForTest(t *testing.T, minShardNo int, maxShardNo int) (int64, int) {
	t.Helper()
	for id := int64(100000); id < 200000; id++ {
		shardNo := idgen.ShardNo(id)
		if shardNo >= minShardNo && shardNo <= maxShardNo {
			return id, shardNo
		}
	}
	t.Fatalf("cannot find test user id in shard range %d-%d", minShardNo, maxShardNo)
	return 0, 0
}

// userSQLiteForTest 使用 SQLite 创建用户物理表，业务读写仍走 User。
type userSQLiteForTest struct {
	ID           int64     `gorm:"column:id;type:integer;primaryKey"`                                                 // 用户 ID
	ShardNo      int       `gorm:"column:shard_no;type:int;not null;index:idx_user_shard_no_id,priority:1"`           // 逻辑分片
	Username     string    `gorm:"column:username;type:varchar(32);not null;uniqueIndex:uk_user_username"`            // 用户名
	Nickname     string    `gorm:"column:nickname;type:varchar(64);not null;default:''"`                              // 昵称
	PasswordHash string    `gorm:"column:password_hash;type:varchar(255);not null"`                                   // 密码哈希
	Email        string    `gorm:"column:email;type:varchar(128);not null;default:'';index:idx_user_email"`           // 邮箱
	Phone        string    `gorm:"column:phone;type:varchar(32);not null;default:'';index:idx_user_phone"`            // 手机号
	Avatar       string    `gorm:"column:avatar;type:varchar(255);not null;default:''"`                               // 头像
	Status       int       `gorm:"column:status;type:tinyint;not null;default:1;index:idx_user_status_id,priority:1"` // 状态
	LastLoginAt  time.Time `gorm:"column:last_login_at;type:datetime"`                                                // 最后登录时间
	LastLoginIP  string    `gorm:"column:last_login_ip;type:varchar(45);not null;default:''"`                         // 最后登录 IP
	CreatedAt    time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                // 创建时间
	UpdatedAt    time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                // 更新时间
}

// userAccountSQLiteForTest 使用 SQLite 创建账号索引表，业务读写仍走 UserAccount。
type userAccountSQLiteForTest struct {
	ID              uint64    `gorm:"column:id;type:integer;primaryKey;autoIncrement:true"`                                                                  // 主键
	Username        string    `gorm:"column:username;type:varchar(32);not null;uniqueIndex:uk_user_account_username"`                                        // 用户名
	UserID          int64     `gorm:"column:user_id;type:integer;not null;uniqueIndex:uk_user_account_user_id;index:idx_user_account_shard_user,priority:2"` // 用户 ID
	ShardNo         int       `gorm:"column:shard_no;type:int;not null;index:idx_user_account_shard_user,priority:1"`                                        // 逻辑分片
	RouteShardCount int       `gorm:"column:route_shard_count;type:int;not null;default:1"`                                                                  // 物理表数量
	CreatedAt       time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                                                    // 创建时间
	UpdatedAt       time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                                                    // 更新时间
}

// TableName 表示测试辅助逻辑。
func (*userAccountSQLiteForTest) TableName() string {
	return TableNameUserAccount
}
