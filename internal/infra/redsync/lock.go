package redsync

import (
	"context"
	stderrors "errors"
	"strings"
	"sync"
	"time"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
	redsyncv4 "github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
)

var (
	// ErrLockLost 表示锁在持有期间已经过期或续期失败，调用方应尽快停止受保护逻辑。
	ErrLockLost = errors.New("Redis 锁已丢失")
	// ErrLockTaken 表示 Redis 锁已被其它任务持有，调用方可按业务语义选择跳过本次触发。
	ErrLockTaken = errors.New("Redis 锁已被占用")
)

const (
	// minLockOperationTimeout 是单次 Redis 锁操作的最短超时时间。
	minLockOperationTimeout = 50 * time.Millisecond
	// maxLockOperationTimeout 是单次 Redis 锁操作的最长超时时间。
	maxLockOperationTimeout = 500 * time.Millisecond
)

// Lock 封装基于 Redis 的分布式互斥锁，并支持后台自动续期。
// 单个 Lock 实例只建议用于一次加锁/释放生命周期。
type Lock struct {
	rs       *redsyncv4.Redsync // redsync 管理器，负责创建底层分布式互斥锁
	mutex    *redsyncv4.Mutex   // 当前生命周期内持有的底层互斥锁实例
	key      string             // Redis 锁 key
	token    string             // 当前持锁实例 token
	ttl      time.Duration      // 当前锁生命周期 TTL
	cancel   context.CancelFunc // 当前锁生命周期取消函数
	done     chan struct{}      // 通知续期 goroutine 停止
	lost     chan error         // 锁丢失通知通道
	opMu     sync.Mutex         // 串行化 Extend/Unlock
	once     sync.Once          // 确保 done 只关闭一次
	lostOnce sync.Once          // 确保 lost 只关闭一次
}

// NewLock 基于传入的 Redis 客户端创建分布式锁实例。
func NewLock(redisClient redis.UniversalClient, key string) *Lock {
	lock := &Lock{
		key:   strings.TrimSpace(key),
		token: utils.RandStr(16, utils.RandSource),
		done:  make(chan struct{}),
		lost:  make(chan error, 1),
	}
	if redisClient != nil {
		pool := goredis.NewPool(redisClient)
		lock.rs = redsyncv4.New(pool)
	}
	return lock
}

// TryLock 尝试获取锁；加锁成功后会自动续期，直到调用 Unlock 或续期失败。
func (l *Lock) TryLock(ctx context.Context, ttl time.Duration) error {
	if l == nil || l.rs == nil {
		return errors.Errorf("Redis 锁未初始化")
	}
	if l.key == "" {
		return errors.Errorf("Redis 锁 key 不能为空")
	}
	if ttl <= 0 {
		return errors.Errorf("Redis 锁 TTL 必须大于 0")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lockCtx, cancel := context.WithCancel(ctx)
	l.ttl = ttl
	l.cancel = cancel

	l.mutex = l.rs.NewMutex(l.key,
		redsyncv4.WithExpiry(ttl),
		redsyncv4.WithTries(5),
		redsyncv4.WithRetryDelayFunc(func(tries int) time.Duration {
			delay := 20 * time.Millisecond
			if tries > 1 {
				delay = delay << (tries - 1)
			}
			if delay > 200*time.Millisecond {
				return 200 * time.Millisecond
			}
			return delay
		}),
		redsyncv4.WithGenValueFunc(func() (string, error) {
			return l.token, nil
		}),
	)

	if err := l.mutex.LockContext(lockCtx); err != nil {
		cancel()
		if isLockTakenError(err) {
			return stderrors.Join(errors.Wrap(err, "获取 Redis 锁失败"), ErrLockTaken)
		}
		return errors.Wrap(err, "获取 Redis 锁失败")
	}

	go l.startRenewal(ttl)
	return nil
}

// IsLockTaken 判断错误是否表示锁被其它节点持有。
func IsLockTaken(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrLockTaken) || isLockTakenError(err)
}

// isLockTakenError 识别 go-redsync 锁竞争错误文本。
func isLockTakenError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "lock already taken")
}

// startRenewal 周期性续期当前锁；一旦续期失败，会通知锁丢失并停止续期。
func (l *Lock) startRenewal(ttl time.Duration) {
	interval := ttl / 2
	if interval <= 0 {
		interval = ttl
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.opMu.Lock()
			if l.mutex == nil || !l.mutex.Until().After(time.Now()) {
				l.reportLoss(ErrLockLost)
				l.opMu.Unlock()
				return
			}
			renewCtx, cancel := context.WithTimeout(context.Background(), lockOperationTimeout(ttl))
			ok, err := l.mutex.ExtendContext(renewCtx)
			cancel()
			if !ok || err != nil {
				if err != nil {
					l.reportLoss(errors.Wrap(err, "续期 Redis 锁失败"))
				} else {
					l.reportLoss(ErrLockLost)
				}
				l.opMu.Unlock()
				return
			}
			l.opMu.Unlock()
		case <-l.done:
			return
		}
	}
}

// Unlock 停止自动续期并释放分布式锁。
func (l *Lock) Unlock() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.done != nil {
			close(l.done)
		}
		if l.cancel != nil {
			l.cancel()
		}
	})
	defer l.closeLost(nil)

	if l.mutex == nil {
		return nil
	}
	l.opMu.Lock()
	defer l.opMu.Unlock()
	unlockCtx, cancel := context.WithTimeout(context.Background(), lockOperationTimeout(l.ttl))
	defer cancel()
	if ok, err := l.mutex.UnlockContext(unlockCtx); !ok || err != nil {
		if err != nil {
			return errors.Wrap(err, "释放 Redis 锁失败")
		}
		return errors.Errorf("释放 Redis 锁失败")
	}
	return nil
}

// lockOperationTimeout 根据锁 TTL 计算单次 Redis 锁操作的保护超时。
func lockOperationTimeout(ttl time.Duration) time.Duration {
	timeout := ttl / 4
	if timeout < minLockOperationTimeout {
		return minLockOperationTimeout
	}
	if timeout > maxLockOperationTimeout {
		return maxLockOperationTimeout
	}
	return timeout
}

// Lost 返回锁丢失通知通道；正常释放时通道会关闭且不会返回错误。
func (l *Lock) Lost() <-chan error {
	if l == nil || l.lost == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return l.lost
}

// reportLoss 记录锁丢失原因，并取消当前锁生命周期上下文。
func (l *Lock) reportLoss(err error) {
	if err == nil {
		err = ErrLockLost
	}
	if l.cancel != nil {
		l.cancel()
	}
	l.closeLost(err)
}

// closeLost 关闭锁丢失通知通道；err 非空时会在关闭前发送给监听方。
func (l *Lock) closeLost(err error) {
	if l == nil || l.lost == nil {
		return
	}
	l.lostOnce.Do(func() {
		if err != nil {
			l.lost <- err
		}
		close(l.lost)
	})
}

// drainError 非阻塞读取错误通道，避免 WithLock 在收尾阶段因为监听协程阻塞。
func drainError(ch <-chan error) error {
	select {
	case err, ok := <-ch:
		if ok {
			return errors.Tag(err)
		}
	default:
	}
	return nil
}

// WithLock 在持有 Redis 分布式锁期间执行 fn。
func WithLock(ctx context.Context, redisClient redis.UniversalClient, key string, ttl time.Duration, fn func(context.Context) error) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	lock := NewLock(redisClient, key)
	if err := lock.TryLock(runCtx, ttl); err != nil {
		return errors.Tag(err)
	}

	renewalErrCh := make(chan error, 1)
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		lostErr, ok := <-lock.Lost()
		if ok && lostErr != nil {
			runCancel()
			renewalErrCh <- lostErr
		}
	}()

	if fn == nil {
		unlockErr := lock.Unlock()
		<-watcherDone
		return errors.Tag(unlockErr)
	}

	err = fn(runCtx)
	if unlockErr := lock.Unlock(); unlockErr != nil {
		err = stderrors.Join(err, unlockErr)
	}
	<-watcherDone
	if lostErr := drainError(renewalErrCh); lostErr != nil {
		err = stderrors.Join(err, lostErr)
	}
	return errors.Tag(err)
}
