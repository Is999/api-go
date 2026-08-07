package user

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"api/common/idgen"
	keys "api/common/rediskeys"
	"api/common/runtimecfg"
	"api/internal/config"
	redislock "api/internal/infra/redsync"
	"api/internal/model"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestGetUserByIDUsesIdentityRoute 确保用户 ID 通过身份目录定位物理表。
func TestGetUserByIDUsesIdentityRoute(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "user-fast-path.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	userID := int64(1)
	for idgen.ShardNo(userID) < 512 {
		userID++
	}
	shardNo := idgen.ShardNo(userID)
	if err = db.Exec("CREATE TABLE user_b0512 (id INTEGER PRIMARY KEY, shard_no INTEGER NOT NULL, username TEXT NOT NULL, status INTEGER NOT NULL, auth_version INTEGER NOT NULL)").Error; err != nil {
		t.Fatalf("create user physical table error = %v", err)
	}
	if err = db.Exec("CREATE TABLE user_identity_username (id INTEGER PRIMARY KEY, identity_value TEXT NOT NULL, user_id INTEGER NOT NULL, user_shard_no INTEGER NOT NULL)").Error; err != nil {
		t.Fatalf("create user identity table error = %v", err)
	}
	if err = db.Exec("INSERT INTO user_b0512 (id, shard_no, username, status, auth_version) VALUES (?, ?, ?, ?, ?)", userID, shardNo, "demo", model.UserStatusEnabled, 7).Error; err != nil {
		t.Fatalf("insert user error = %v", err)
	}
	if err = db.Exec("INSERT INTO user_identity_username (id, identity_value, user_id, user_shard_no) VALUES (?, ?, ?, ?)", 1, "demo", userID, shardNo).Error; err != nil {
		t.Fatalf("insert user identity error = %v", err)
	}

	split := NewUserLogic(context.Background(), svc.NewServiceContext(config.Config{
		User: config.UserConfig{RouteShardCount: 2},
	}, "v1", svc.Dependencies{}))
	splitUser, err := split.getUserByID(db, userID)
	if err != nil {
		t.Fatalf("split getUserByID() error = %v", err)
	}
	if splitUser == nil || splitUser.ID != userID || splitUser.Username != "demo" || splitUser.AuthVersion != 7 {
		t.Fatalf("split user = %+v, want id=%d", splitUser, userID)
	}
}

// TestUserProfileRebuildWaitDelay 校验缓存观察间隔线性增长、封顶和抖动边界。
func TestUserProfileRebuildWaitDelay(t *testing.T) {
	// cases 覆盖每轮上下界以及非法参数归一化，防止缓存竞争路径退化为固定高频轮询。
	cases := []struct {
		name    string        // 当前边界场景
		attempt int           // 第几段等待，从 1 开始
		jitter  time.Duration // 测试注入的抖动
		want    time.Duration // 归一化后的最终等待
	}{
		{name: "first lower", attempt: 1, jitter: 0, want: 50 * time.Millisecond},
		{name: "first upper", attempt: 1, jitter: 50 * time.Millisecond, want: 100 * time.Millisecond},
		{name: "second lower", attempt: 2, jitter: 0, want: 100 * time.Millisecond},
		{name: "third upper", attempt: 3, jitter: 50 * time.Millisecond, want: 200 * time.Millisecond},
		{name: "fifth lower", attempt: 5, jitter: 0, want: 250 * time.Millisecond},
		{name: "base capped", attempt: 6, jitter: 0, want: 250 * time.Millisecond},
		{name: "jitter capped", attempt: 6, jitter: time.Second, want: 300 * time.Millisecond},
		{name: "invalid values", attempt: 0, jitter: -time.Second, want: 50 * time.Millisecond},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := userProfileRebuildWaitDelay(tt.attempt, tt.jitter); got != tt.want {
				t.Fatalf("userProfileRebuildWaitDelay(%d, %v) = %v, want %v", tt.attempt, tt.jitter, got, tt.want)
			}
		})
	}

	var minTotal time.Duration
	var maxTotal time.Duration
	for attempt := 1; attempt <= userProfileRebuildWaitAttempts; attempt++ {
		minTotal += userProfileRebuildWaitDelay(attempt, 0)
		maxTotal += userProfileRebuildWaitDelay(attempt, userProfileRebuildWaitJitter)
	}
	if minTotal != 750*time.Millisecond || maxTotal != time.Second {
		t.Fatalf("cache observation delay total = %v–%v, want 750ms–1s", minTotal, maxTotal)
	}
}

// TestGetUserProfileCollapsesConcurrentCacheMiss 验证并发缓存未命中只执行一次身份目录和物理表回源。
func TestGetUserProfileCollapsesConcurrentCacheMiss(t *testing.T) {
	const (
		appID           = "profile-singleflight"
		concurrency     = 16
		routeShardCount = 2
	)
	previousRuntime := runtimecfg.Get()
	runtimecfg.Set(config.Config{AppID: appID})
	t.Cleanup(func() {
		runtimecfg.Restore(previousRuntime)
	})

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "user-profile.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	userID := int64(1)
	for idgen.ShardNo(userID) < 512 {
		userID++
	}
	shardNo := idgen.ShardNo(userID)
	if err = db.Exec("CREATE TABLE user_b0512 (id INTEGER PRIMARY KEY, shard_no INTEGER NOT NULL, username TEXT NOT NULL, status INTEGER NOT NULL, auth_version INTEGER NOT NULL)").Error; err != nil {
		t.Fatalf("create user physical table error = %v", err)
	}
	if err = db.Exec("CREATE TABLE user_identity_username (id INTEGER PRIMARY KEY, identity_value TEXT NOT NULL, user_id INTEGER NOT NULL, user_shard_no INTEGER NOT NULL)").Error; err != nil {
		t.Fatalf("create user identity table error = %v", err)
	}
	if err = db.Exec("INSERT INTO user_b0512 (id, shard_no, username, status, auth_version) VALUES (?, ?, ?, ?, ?)", userID, shardNo, "demo", model.UserStatusEnabled, 7).Error; err != nil {
		t.Fatalf("insert user error = %v", err)
	}
	if err = db.Exec("INSERT INTO user_identity_username (id, identity_value, user_id, user_shard_no) VALUES (?, ?, ?, ?)", 1, "demo", userID, shardNo).Error; err != nil {
		t.Fatalf("insert user identity error = %v", err)
	}

	var queryCount atomic.Int32
	if err = db.Callback().Query().Before("gorm:query").Register("test:count_profile_query", func(*gorm.DB) {
		queryCount.Add(1)
		time.Sleep(100 * time.Millisecond)
	}); err != nil {
		t.Fatalf("register query callback error = %v", err)
	}
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	service := svc.NewServiceContext(config.Config{
		AppID: appID,
		Auth:  config.AuthConfig{ProfileCacheTTLSeconds: 300},
		User:  config.UserConfig{RouteShardCount: routeShardCount},
	}, "v1", svc.Dependencies{
		SiteDBs: svc.SiteDatabases{MainDB: db},
		Rds:     client,
	})

	start := make(chan struct{})
	errs := make(chan error, concurrency)
	var waitGroup sync.WaitGroup
	waitGroup.Add(concurrency)
	for range concurrency {
		go func() {
			defer waitGroup.Done()
			<-start
			profile, err := NewUserLogic(context.Background(), service).GetUserProfile(userID)
			if err == nil && (profile == nil || profile.ID != userID) {
				err = errors.Errorf("profile = %+v, want user_id=%d", profile, userID)
			}
			errs <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("GetUserProfile() error = %v", err)
		}
	}
	if got := queryCount.Load(); got != 2 {
		t.Fatalf("profile query count = %d, want 2", got)
	}

	logic := NewUserLogic(context.Background(), service)
	cacheKey := logic.userProfileKey(userID)
	if ttl := server.TTL(cacheKey); ttl < 299*time.Second || ttl > 330*time.Second {
		t.Fatalf("profile cache TTL = %s, want about 300s with at most 10%% jitter", ttl)
	}
	if err := logic.DeleteUserProfileCache(userID); err != nil {
		t.Fatalf("DeleteUserProfileCache() error = %v", err)
	}
	lock := redislock.NewLock(client, logic.userProfileRebuildLockKey(userID))
	if err := lock.TryLock(context.Background(), userProfileRebuildLockTTL); err != nil {
		t.Fatalf("TryLock() error = %v", err)
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			_ = lock.Unlock()
		}
	}()

	queryCountBeforeWait := queryCount.Load()
	commandsBeforeWait := server.CommandCount()
	waitErr := make(chan error, 1)
	go func() {
		profile, err := logic.loadUserProfile(cacheKey, userID)
		if err == nil && (profile == nil || profile.ID != userID) {
			err = errors.Errorf("profile = %+v, want user_id=%d", profile, userID)
		}
		waitErr <- err
	}()
	time.Sleep(450 * time.Millisecond)
	if err := logic.CacheUserProfile(userID, BuildUserProfile(&model.User{
		ID:       userID,
		ShardNo:  shardNo,
		Username: "demo",
		Status:   model.UserStatusEnabled,
	})); err != nil {
		t.Fatalf("CacheUserProfile() error = %v", err)
	}
	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatalf("loadUserProfile() while lock is held error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loadUserProfile() did not observe the cache rebuilt by another process")
	}
	if got := queryCount.Load(); got != queryCountBeforeWait {
		t.Fatalf("profile query count during distributed lock contention = %d, want %d", got, queryCountBeforeWait)
	}
	// miniredis 的单轮冷启动锁路径最多计 4 条命令，随后最多六次 GET，测试写回再占一次 SET；上限 12 可防止恢复双重轮询。
	if got := server.CommandCount() - commandsBeforeWait; got > 12 {
		t.Fatalf("Redis command count during distributed lock contention = %d, want <= 12", got)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	lockHeld = false
}

// TestGetUserProfileCachesMissingUser 验证不存在用户使用短 TTL 空值缓存并可被正值覆盖。
func TestGetUserProfileCachesMissingUser(t *testing.T) {
	const (
		appID           = "profile-empty-cache"
		routeShardCount = 2
		userID          = int64(42)
	)
	previousRuntime := runtimecfg.Get()
	runtimecfg.Set(config.Config{AppID: appID})
	t.Cleanup(func() {
		runtimecfg.Restore(previousRuntime)
	})

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "missing-user-profile.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err = db.Exec("CREATE TABLE user_identity_username (id INTEGER PRIMARY KEY, identity_value TEXT NOT NULL, user_id INTEGER NOT NULL, user_shard_no INTEGER NOT NULL)").Error; err != nil {
		t.Fatalf("create user identity table error = %v", err)
	}
	var queryCount atomic.Int32
	if err = db.Callback().Query().Before("gorm:query").Register("test:count_missing_profile_query", func(*gorm.DB) {
		queryCount.Add(1)
	}); err != nil {
		t.Fatalf("register query callback error = %v", err)
	}
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	service := svc.NewServiceContext(config.Config{
		AppID: appID,
		Auth:  config.AuthConfig{ProfileCacheTTLSeconds: 300},
		User:  config.UserConfig{RouteShardCount: routeShardCount},
	}, "v1", svc.Dependencies{
		SiteDBs: svc.SiteDatabases{MainDB: db},
		Rds:     client,
	})
	logic := NewUserLogic(context.Background(), service)

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := logic.GetUserProfile(userID); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("GetUserProfile() attempt %d error = %v, want ErrUserNotFound", attempt+1, err)
		}
	}
	if got := queryCount.Load(); got != 1 {
		t.Fatalf("missing profile query count = %d, want 1", got)
	}
	cacheKey := logic.userProfileKey(userID)
	if value, err := client.Get(context.Background(), cacheKey).Result(); err != nil || value != keys.EmptyValueMarker {
		t.Fatalf("missing profile cache = %q, %v; want empty marker", value, err)
	}
	if ttl := server.TTL(cacheKey); ttl < 119*time.Second || ttl > 132*time.Second {
		t.Fatalf("missing profile cache TTL = %s, want about 120s with at most 10%% jitter", ttl)
	}

	if err := logic.CacheUserProfile(userID, BuildUserProfile(&model.User{
		ID:       userID,
		ShardNo:  idgen.ShardNo(userID),
		Username: "created-user",
		Status:   model.UserStatusEnabled,
	})); err != nil {
		t.Fatalf("CacheUserProfile() error = %v", err)
	}
	profile, err := logic.GetUserProfile(userID)
	if err != nil || profile == nil || profile.Username != "created-user" {
		t.Fatalf("GetUserProfile() after positive cache = %+v, %v", profile, err)
	}
	if got := queryCount.Load(); got != 1 {
		t.Fatalf("profile query count after positive overwrite = %d, want 1", got)
	}
}
