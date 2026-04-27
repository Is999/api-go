package health

import (
	"context"
	"os"
	"strings"
	"time"

	codes "api/common/codes"
	"api/internal/config"
	corelogic "api/internal/logic"
	"api/internal/svc"
	"api/internal/types"

	"github.com/Is999/go-utils/errors"
)

// 健康检查固定状态和超时阈值。
const (
	healthCheckTimeout = 2 * time.Second
	healthStatusOK     = "ok"
	healthStatusError  = "error"
)

// HealthLogic 负责 live/ready 健康检查。
type HealthLogic struct {
	*corelogic.BaseLogic // BaseLogic 提供统一上下文、日志和 ServiceContext 访问能力。
}

// NewHealthLogic 创建健康检查 logic。
func NewHealthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HealthLogic {
	return &HealthLogic{BaseLogic: corelogic.NewBaseLogicWithContext(ctx, svcCtx)}
}

// Liveness 返回进程存活状态。
func (l *HealthLogic) Liveness() *types.HealthStatusResp {
	return &types.HealthStatusResp{
		Status:  healthStatusOK,
		Mode:    "api",
		Node:    l.nodeName(),
		Version: l.currentVersion(),
	}
}

// Readiness 检查核心依赖是否可用。
func (l *HealthLogic) Readiness(ctx context.Context) (*types.HealthStatusResp, error) {
	statuses := make([]types.HealthDependencyStatus, 0, 4)
	var firstErr error
	appendStatus := func(status types.HealthDependencyStatus, err error) {
		statuses = append(statuses, status)
		if err != nil && firstErr == nil {
			firstErr = errors.Tag(err)
		}
	}

	if l.service() == nil {
		appendStatus(dependencyError("service_context", codes.DependencyUnavailable, errors.Errorf("ServiceContext未初始化")))
	} else {
		components := l.service().ComponentRegistry()
		items := components.Items()
		if len(items) == 0 {
			appendStatus(dependencyError("component_registry", codes.DependencyUnavailable, errors.Errorf("组件生命周期注册表未初始化")))
		}
		for _, component := range items {
			appendStatus(l.checkComponent(ctx, component))
		}
	}

	resp := &types.HealthStatusResp{
		Status:       healthStatusOK,
		Mode:         "api",
		Node:         l.nodeName(),
		Version:      l.currentVersion(),
		Dependencies: statuses,
	}
	if firstErr != nil {
		resp.Status = healthStatusError
		return resp, firstErr
	}
	return resp, nil
}

// currentConfig 返回健康检查使用的当前配置快照。
func (l *HealthLogic) currentConfig() config.Config {
	if l == nil || l.service() == nil {
		return config.Config{}
	}
	return l.service().CurrentConfig()
}

// currentVersion 返回当前配置版本，缺省时显示 unknown。
func (l *HealthLogic) currentVersion() string {
	if l == nil || l.service() == nil {
		return "unknown"
	}
	version := strings.TrimSpace(l.service().CurrentVersion())
	if version == "" {
		return "unknown"
	}
	return version
}

// nodeName 优先使用配置实例 ID，缺省时回退主机名。
func (l *HealthLogic) nodeName() string {
	cfg := l.currentConfig()
	if strings.TrimSpace(cfg.InstanceID) != "" {
		return strings.TrimSpace(cfg.InstanceID)
	}
	if name, err := os.Hostname(); err == nil && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "unknown"
}

// service 安全返回 ServiceContext，避免健康检查空指针。
func (l *HealthLogic) service() *svc.ServiceContext {
	if l == nil || l.Svc == nil {
		return nil
	}
	return l.Svc
}

// checkComponent 在受控超时时间内探测单个注册组件。
func (l *HealthLogic) checkComponent(ctx context.Context, component svc.Component) (types.HealthDependencyStatus, error) {
	name := strings.TrimSpace(component.Name)
	if name == "" {
		name = "unknown"
	}
	if component.Check == nil {
		return dependencyOK(name), nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	checkCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()
	if err := component.Check(checkCtx); err != nil {
		code := component.ErrorCode
		if code == 0 {
			code = codes.DependencyUnavailable
		}
		return dependencyError(name, code, err)
	}
	return dependencyOK(name), nil
}

// dependencyOK 构造 ready 依赖正常状态。
func dependencyOK(name string) types.HealthDependencyStatus {
	return types.HealthDependencyStatus{Name: name, Status: healthStatusOK}
}

// dependencyError 构造 ready 依赖异常状态和可追踪错误。
func dependencyError(name string, code int, err error) (types.HealthDependencyStatus, error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	status := types.HealthDependencyStatus{Name: name, Status: healthStatusError, Code: code, Message: message}
	return status, errors.Wrapf(err, "ready依赖检查失败 name=%s code=%d", name, code)
}
