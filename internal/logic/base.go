package logic

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gozero_api/helper"
	"gozero_api/internal/infra/loggerx"
	"gozero_api/internal/requestctx"
	"gozero_api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

// BaseLogic 是所有业务 logic 的公共基座。
type BaseLogic struct {
	logx.Logger                     // 已绑定当前请求上下文的日志记录器
	ctx         context.Context     // 当前 logic 处理链路使用的上下文
	svc         *svc.ServiceContext // 绑定当前上下文后的服务依赖集合
}

// NewBaseLogic 兼容现有以 http.Request 创建 logic 的调用方式。
func NewBaseLogic(r *http.Request, svcCtx *svc.ServiceContext) *BaseLogic {
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	return NewBaseLogicWithContext(ctx, svcCtx)
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
		ctx:    ctx,
		svc:    scopedSvc,
	}
}

// Context 返回当前 logic 绑定的请求上下文。
func (l *BaseLogic) Context() context.Context {
	return l.ctx
}

// Redis 返回共享 Redis 客户端。
func (l *BaseLogic) Redis() redis.UniversalClient {
	if l.svc == nil {
		return nil
	}
	return l.svc.Rds
}

// Meta 返回当前请求链路元数据。
func (l *BaseLogic) Meta() *requestctx.Meta {
	return requestctx.FromContext(l.ctx)
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
	user := helper.GetCtxUser(l.ctx)
	if user == nil {
		return &helper.CtxUser{}
	}
	return user
}

// RdsGetJsonObj 从 Redis 读取 JSON 字符串并反序列化到目标对象。
func (l *BaseLogic) RdsGetJsonObj(key string, dest any) error {
	if l == nil || l.svc == nil || l.svc.Rds == nil {
		return errors.New("Redis 未初始化")
	}
	val, err := l.svc.Rds.Get(l.ctx, key).Result()
	if err != nil {
		return errors.Tag(err)
	}
	return errors.Tag(json.Unmarshal([]byte(val), dest))
}

// RdsSetJSONValue 将值序列化为 JSON 后写入 Redis。
func (l *BaseLogic) RdsSetJSONValue(key string, value any, expireSec int64) error {
	if l == nil || l.svc == nil || l.svc.Rds == nil {
		return errors.New("Redis 未初始化")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return errors.Tag(err)
	}
	return errors.Tag(l.svc.Rds.Set(l.ctx, key, data, jitterTTL(time.Duration(expireSec)*time.Second)).Err())
}

// RdsDelKeys 批量删除 Redis 键。
func (l *BaseLogic) RdsDelKeys(keys ...string) error {
	if l == nil || l.svc == nil || l.svc.Rds == nil {
		return errors.New("Redis 未初始化")
	}
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized = append(normalized, key)
	}
	if len(normalized) == 0 {
		return nil
	}
	return errors.Tag(l.svc.Rds.Del(l.ctx, normalized...).Err())
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
