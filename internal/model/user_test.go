package model

import (
	"fmt"
	"testing"
	"time"

	"api/common/idgen"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testUserPrivacySecret = "test-user-privacy-secret"

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

// TestUserIdentityTableName 验证身份类型稳定路由到独立物理表。
func TestUserIdentityTableName(t *testing.T) {
	tests := []struct {
		identityType string // identityType 表示身份类型。
		want         string // want 表示期望物理表。
	}{
		{identityType: UserIdentityTypeUsername, want: TableNameUserIdentityUsername},
		{identityType: UserIdentityTypeEmail, want: TableNameUserIdentityEmail},
		{identityType: UserIdentityTypePhone, want: TableNameUserIdentityPhone},
		{identityType: UserIdentityTypeOAuth, want: TableNameUserIdentityOAuth},
	}
	for _, tt := range tests {
		t.Run(tt.identityType, func(t *testing.T) {
			got, err := UserIdentityTableName(tt.identityType)
			if err != nil {
				t.Fatalf("UserIdentityTableName() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UserIdentityTableName() = %q, want %q", got, tt.want)
			}
		})
	}
	if _, err := UserIdentityTableName("unknown"); err == nil {
		t.Fatal("期望非法身份类型返回错误")
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

// TestUserIdentityTableNameRejectsMismatchedShardNo 验证身份索引不会接受错误分片号。
func TestUserIdentityTableNameRejectsMismatchedShardNo(t *testing.T) {
	userID := int64(123456789)
	identity := &UserIdentity{
		IdentityType:        UserIdentityTypeUsername,
		Provider:            UserIdentityProviderLocal,
		IdentityValue:       "demo_user",
		UserID:              userID,
		UserShardNo:         idgen.ShardNo(userID),
		UserRouteShardCount: 1024,
	}
	want, err := UserPhysicalTableName(identity.UserShardNo, identity.UserRouteShardCount)
	if err != nil {
		t.Fatalf("UserPhysicalTableName() error = %v", err)
	}
	got, err := identity.UserTableName()
	if err != nil {
		t.Fatalf("UserTableName() error = %v", err)
	}
	if got != want {
		t.Fatalf("UserTableName() = %q, want %q", got, want)
	}

	identity.UserShardNo = (identity.UserShardNo + 1) % userRouteShardMod
	if _, err := identity.UserTableName(); err == nil {
		t.Fatal("期望身份索引 user_shard_no 与 user_id 不一致时返回错误")
	}
}

// TestSafeUserUpdatesRejectsImmutableFields 验证通用更新不会修改用户分片和唯一账号字段。
func TestSafeUserUpdatesRejectsImmutableFields(t *testing.T) {
	got := safeUserUpdates(map[string]any{
		"id":            int64(1),
		"shard_no":      12,
		"username":      "changed",
		"password_hash": "unsafe",
		"email":         "raw@example.com",
		"email_hash":    " hash ",
	}, false)
	for _, key := range []string{"id", "shard_no", "username", "password_hash", "email"} {
		if _, ok := got[key]; ok {
			t.Fatalf("safeUserUpdates() should reject %s: %+v", key, got)
		}
	}
	if got["email_hash"] != "hash" {
		t.Fatalf("safeUserUpdates() should keep secure email fields: %+v", got)
	}
}

// TestCreateUserWithIdentitiesRoutesPhysicalTable 验证非默认物理表数量会真实写入并路由读取。
func TestCreateUserWithIdentitiesRoutesPhysicalTable(t *testing.T) {
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
	if err = migrateUserIdentityTablesForTest(db); err != nil {
		t.Fatalf("migrateUserIdentityTablesForTest() error = %v", err)
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
		Email:        "Route_User@Example.Test",
		Phone:        "19900000001",
		Status:       UserStatusEnabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err = CreateUserWithIdentities(db, user, 4, testUserPrivacySecret); err != nil {
		t.Fatalf("CreateUserWithIdentities() error = %v", err)
	}
	var count int64
	if err = db.Table(tableName).Where("id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count routed user error = %v", err)
	}
	if count != 1 {
		t.Fatalf("routed table count = %d, want 1", count)
	}
	got, err := FindUserByIdentity(db, UserIdentityTypeUsername, UserIdentityProviderLocal, "route_user", testUserPrivacySecret)
	if err != nil {
		t.Fatalf("FindUserByIdentity(username) error = %v", err)
	}
	if got == nil || got.ID != userID {
		t.Fatalf("FindUserByIdentity(username) = %+v, want id=%d", got, userID)
	}
	identity, err := FindUserIdentity(db, UserIdentityTypeEmail, UserIdentityProviderLocal, "route_user@example.test", testUserPrivacySecret)
	if err != nil {
		t.Fatalf("FindUserIdentity(email) error = %v", err)
	}
	if identity == nil || identity.IdentityHash != user.EmailHash || identity.UserRouteShardCount != 4 || identity.UserShardNo != shardNo || identity.UserID != userID {
		t.Fatalf("identity = %+v, want user=%d shard=%d route=4", identity, userID, shardNo)
	}
	var emailCount int64
	if err = db.Table(TableNameUserIdentityEmail).Where("user_id = ?", userID).Count(&emailCount).Error; err != nil {
		t.Fatalf("count email identity error = %v", err)
	}
	if emailCount != 1 {
		t.Fatalf("email identity table count = %d, want 1", emailCount)
	}
	if err = UpdateUserProfileWithIdentities(db, userID, map[string]any{"email": "changed@example.test"}, testUserPrivacySecret); err != nil {
		t.Fatalf("UpdateUserProfileWithIdentities() error = %v", err)
	}
	if oldIdentity, err := FindUserIdentity(db, UserIdentityTypeEmail, UserIdentityProviderLocal, "route_user@example.test", testUserPrivacySecret); err != nil || oldIdentity != nil {
		t.Fatalf("old email identity = %+v err=%v, want nil", oldIdentity, err)
	}
	if newIdentity, err := FindUserIdentity(db, UserIdentityTypeEmail, UserIdentityProviderLocal, "changed@example.test", testUserPrivacySecret); err != nil || newIdentity == nil || newIdentity.UserID != userID {
		t.Fatalf("new email identity = %+v err=%v, want user=%d", newIdentity, err, userID)
	}
}

// TestUpdateUserProfileWithIdentitiesRollsBackOnIdentityConflict 验证身份索引冲突时主表资料同步回滚。
func TestUpdateUserProfileWithIdentitiesRollsBackOnIdentityConflict(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:user_identity_conflict_test?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err = migrateUserIdentityTablesForTest(db); err != nil {
		t.Fatalf("migrateUserIdentityTablesForTest() error = %v", err)
	}
	if err = db.Table(TableNameUser).AutoMigrate(&userSQLiteForTest{}); err != nil {
		t.Fatalf("AutoMigrate(user) error = %v", err)
	}
	firstUser := userForIdentityConflictTest(100001, "identity_user_001", "first@example.test")
	secondUser := userForIdentityConflictTest(100002, "identity_user_002", "second@example.test")
	if err = CreateUserWithIdentities(db, firstUser, UserRouteShardCountDefault, testUserPrivacySecret); err != nil {
		t.Fatalf("CreateUserWithIdentities(first) error = %v", err)
	}
	if err = CreateUserWithIdentities(db, secondUser, UserRouteShardCountDefault, testUserPrivacySecret); err != nil {
		t.Fatalf("CreateUserWithIdentities(second) error = %v", err)
	}
	err = UpdateUserProfileWithIdentities(db, firstUser.ID, map[string]any{"email": secondUser.Email}, testUserPrivacySecret)
	if err == nil {
		t.Fatal("期望邮箱身份冲突时返回错误")
	}
	got, err := FindUserByID(db, firstUser.ID)
	if err != nil {
		t.Fatalf("FindUserByID(first) error = %v", err)
	}
	if got == nil || got.EmailHash != firstUser.EmailHash || got.EmailMasked != firstUser.EmailMasked {
		t.Fatalf("first user secure email = %+v, want hash=%s masked=%s", got, firstUser.EmailHash, firstUser.EmailMasked)
	}
	firstIdentity, err := FindUserIdentity(db, UserIdentityTypeEmail, UserIdentityProviderLocal, firstUser.Email, testUserPrivacySecret)
	if err != nil {
		t.Fatalf("FindUserIdentity(first email) error = %v", err)
	}
	if firstIdentity == nil || firstIdentity.UserID != firstUser.ID {
		t.Fatalf("first email identity = %+v, want user=%d", firstIdentity, firstUser.ID)
	}
	secondIdentity, err := FindUserIdentity(db, UserIdentityTypeEmail, UserIdentityProviderLocal, secondUser.Email, testUserPrivacySecret)
	if err != nil {
		t.Fatalf("FindUserIdentity(second email) error = %v", err)
	}
	if secondIdentity == nil || secondIdentity.UserID != secondUser.ID {
		t.Fatalf("second email identity = %+v, want user=%d", secondIdentity, secondUser.ID)
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

// userForIdentityConflictTest 构造身份冲突测试用户。
func userForIdentityConflictTest(id int64, username string, email string) *User {
	now := time.Now()
	return &User{
		ID:           id,
		ShardNo:      idgen.ShardNo(id),
		Username:     username,
		Nickname:     username,
		PasswordHash: "hash",
		Email:        email,
		Status:       UserStatusEnabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// migrateUserIdentityTablesForTest 创建四张登录身份索引物理表。
func migrateUserIdentityTablesForTest(db *gorm.DB) error {
	for _, tableName := range []string{
		TableNameUserIdentityUsername,
		TableNameUserIdentityEmail,
		TableNameUserIdentityPhone,
		TableNameUserIdentityOAuth,
	} {
		if err := db.Table(tableName).AutoMigrate(&userIdentitySQLiteForTest{}); err != nil {
			return err
		}
		if err := createUserIdentitySQLiteIndexes(db, tableName); err != nil {
			return err
		}
	}
	return nil
}

// createUserIdentitySQLiteIndexes 使用表名前缀规避 SQLite 全库索引名唯一限制。
func createUserIdentitySQLiteIndexes(db *gorm.DB, tableName string) error {
	var statements []string
	switch tableName {
	case TableNameUserIdentityUsername:
		statements = []string{
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_identity_value` ON `%s` (`identity_value`)", tableName, tableName),
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_user` ON `%s` (`user_id`)", tableName, tableName),
		}
	case TableNameUserIdentityEmail, TableNameUserIdentityPhone:
		statements = []string{
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_identity_hash` ON `%s` (`identity_hash`)", tableName, tableName),
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_user` ON `%s` (`user_id`)", tableName, tableName),
		}
	case TableNameUserIdentityOAuth:
		statements = []string{
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_provider_value` ON `%s` (`provider`, `identity_value`)", tableName, tableName),
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_user_provider` ON `%s` (`user_id`, `provider`)", tableName, tableName),
		}
	default:
		return fmt.Errorf("unknown identity table %s", tableName)
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

// userSQLiteForTest 使用 SQLite 创建用户物理表，业务读写仍走 User。
type userSQLiteForTest struct {
	ID              int64     `gorm:"column:id;type:integer;primaryKey"`                                                 // 用户 ID
	ShardNo         int       `gorm:"column:shard_no;type:int;not null;index:idx_user_shard_no_id,priority:1"`           // 逻辑分片
	Username        string    `gorm:"column:username;type:varchar(32);not null;uniqueIndex:uk_user_username"`            // 用户名
	Nickname        string    `gorm:"column:nickname;type:varchar(64);not null;default:''"`                              // 昵称
	PasswordHash    string    `gorm:"column:password_hash;type:varchar(255);not null"`                                   // 密码哈希
	EmailCiphertext string    `gorm:"column:email_ciphertext;type:varchar(512);not null;default:''"`                     // 邮箱密文
	EmailHash       string    `gorm:"column:email_hash;type:char(64);not null;default:'';index:idx_user_email_hash"`     // 邮箱查询哈希
	EmailMasked     string    `gorm:"column:email_masked;type:varchar(128);not null;default:''"`                         // 邮箱脱敏展示值
	EmailKeyVersion string    `gorm:"column:email_key_version;type:varchar(32);not null;default:''"`                     // 邮箱密钥版本
	PhoneCiphertext string    `gorm:"column:phone_ciphertext;type:varchar(512);not null;default:''"`                     // 手机号密文
	PhoneHash       string    `gorm:"column:phone_hash;type:char(64);not null;default:'';index:idx_user_phone_hash"`     // 手机号查询哈希
	PhoneMasked     string    `gorm:"column:phone_masked;type:varchar(32);not null;default:''"`                          // 手机号脱敏展示值
	PhoneKeyVersion string    `gorm:"column:phone_key_version;type:varchar(32);not null;default:''"`                     // 手机号密钥版本
	Avatar          string    `gorm:"column:avatar;type:varchar(255);not null;default:''"`                               // 头像
	Status          int       `gorm:"column:status;type:tinyint;not null;default:1;index:idx_user_status_id,priority:1"` // 状态
	LastLoginAt     time.Time `gorm:"column:last_login_at;type:datetime"`                                                // 最后登录时间
	LastLoginIP     string    `gorm:"column:last_login_ip;type:varchar(45);not null;default:''"`                         // 最后登录 IP
	CreatedAt       time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                // 创建时间
	UpdatedAt       time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                // 更新时间
}

// userIdentitySQLiteForTest 使用 SQLite 创建身份索引表，业务读写仍走 UserIdentity。
type userIdentitySQLiteForTest struct {
	ID                  uint64    `gorm:"column:id;type:integer;primaryKey;autoIncrement:true"`               // 主键
	Provider            string    `gorm:"column:provider;type:varchar(32);not null;default:''"`               // 三方提供方
	IdentityValue       string    `gorm:"column:identity_value;type:varchar(191);not null;default:''"`        // 身份值
	IdentityHash        string    `gorm:"column:identity_hash;type:char(64);not null;default:''"`             // 联系方式身份哈希
	UserID              int64     `gorm:"column:user_id;type:integer;not null"`                               // 用户 ID
	UserShardNo         int       `gorm:"column:user_shard_no;type:int;not null"`                             // 用户分片
	UserRouteShardCount int       `gorm:"column:user_route_shard_count;type:int;not null;default:1"`          // 用户物理表数量
	CreatedAt           time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP"` // 创建时间
	UpdatedAt           time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP"` // 更新时间
}

// TableName 表示测试辅助逻辑。
func (*userIdentitySQLiteForTest) TableName() string {
	return TableNameUserIdentityUsername
}
