package logic

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	keys "api/common/rediskeys"
	"api/helper"
	"api/internal/infra/loggerx"
	"api/internal/requestctx"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

// BaseLogic 是所有业务 logic 的公共基座。
type BaseLogic struct {
	logx.Logger                     // 已绑定当前请求上下文的日志记录器
	Ctx         context.Context     // 当前 logic 处理链路使用的上下文
	Svc         *svc.ServiceContext // 绑定当前上下文后的服务依赖集合
}

// NewBaseLogicWithContext 为当前请求克隆一份带上下文的 ServiceContext。
func NewBaseLogicWithContext(ctx context.Context, svcCtx *svc.ServiceContext) *BaseLogic {
	ctx, _ = requestctx.New(ctx)
	ctx = loggerx.BindContext(ctx)
	var scopedSvc *svc.ServiceContext
	if svcCtx != nil {
		scopedSvc = svcCtx.ScopedWithContext(ctx)
	}
	return &BaseLogic{
		Logger: logx.WithContext(ctx),
		Ctx:    ctx,
		Svc:    scopedSvc,
	}
}

// Redis 返回共享 Redis 客户端。
func (l *BaseLogic) Redis() redis.UniversalClient {
	if l.Svc == nil {
		return nil
	}
	return l.Svc.Rds
}

// AppID 返回当前 Redis 缓存命名空间使用的 app_id。
func (l *BaseLogic) AppID() string {
	if l == nil || l.Svc == nil {
		return ""
	}
	return keys.NormalizeAppID(l.Svc.CurrentConfig().AppID)
}

// AppRedisKey 给业务 Redis key 追加当前 app_id 命名空间。
func (l *BaseLogic) AppRedisKey(key string) string {
	if l == nil {
		return ""
	}
	return keys.AppScopedKey(l.AppID(), key)
}

// Meta 返回当前请求链路元数据。
func (l *BaseLogic) Meta() *requestctx.Meta {
	return requestctx.FromContext(l.Ctx)
}

// ClientIP 返回当前请求的客户端 IP。
func (l *BaseLogic) ClientIP() string {
	if meta := l.Meta(); meta != nil {
		return meta.ClientIP
	}
	return ""
}

// AccessToken 返回当前请求的访问令牌。
func (l *BaseLogic) AccessToken() string {
	if meta := l.Meta(); meta != nil {
		return meta.AccessToken
	}
	return ""
}

// GetCtxUser 返回当前请求上下文中的前台用户信息。
func (l *BaseLogic) GetCtxUser() *helper.CtxUser {
	user := helper.GetCtxUser(l.Ctx)
	if user == nil {
		return &helper.CtxUser{}
	}
	return user
}

// RdsGetJsonObj 从当前 app_id 命名空间读取 JSON 字符串并反序列化到目标对象。
func (l *BaseLogic) RdsGetJsonObj(key string, dest any) error {
	if l == nil || l.Svc == nil || l.Svc.Rds == nil {
		return errors.New("Redis 未初始化")
	}
	key = l.AppRedisKey(key)
	val, err := l.Svc.Rds.Get(l.Ctx, key).Result()
	if err != nil {
		return errors.Tag(err)
	}
	return errors.Tag(json.Unmarshal([]byte(val), dest))
}

// RdsSetJSONValue 将值序列化为 JSON 后写入当前 app_id 命名空间。
func (l *BaseLogic) RdsSetJSONValue(key string, value any, expireSec int64) error {
	if l == nil || l.Svc == nil || l.Svc.Rds == nil {
		return errors.New("Redis 未初始化")
	}
	key = l.AppRedisKey(key)
	data, err := json.Marshal(value)
	if err != nil {
		return errors.Tag(err)
	}
	return errors.Tag(l.Svc.Rds.Set(l.Ctx, key, data, jitterTTL(time.Duration(expireSec)*time.Second)).Err())
}

// RdsDelKeys 批量删除当前 app_id 命名空间下的 Redis 键。
func (l *BaseLogic) RdsDelKeys(keys ...string) error {
	if l == nil || l.Svc == nil || l.Svc.Rds == nil {
		return errors.New("Redis 未初始化")
	}
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		key = l.AppRedisKey(key)
		if key == "" {
			continue
		}
		normalized = append(normalized, key)
	}
	if len(normalized) == 0 {
		return nil
	}
	return errors.Tag(l.Svc.Rds.Del(l.Ctx, normalized...).Err())
}

// wrapLogicError 给业务错误补充调用点上下文。
func wrapLogicError(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	format = strings.TrimSpace(format)
	if format == "" {
		return errors.Tag(err)
	}
	if len(args) > 0 {
		return errors.Wrapf(err, format, args...)
	}
	return errors.Wrap(err, format)
}

// WrapLogicError 给业务错误补充调用点上下文。
func WrapLogicError(err error, format string, args ...any) error {
	return wrapLogicError(err, format, args...)
}

// FormatDateTime 将时间格式化为前端稳定展示字符串。
func FormatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateTime)
}
