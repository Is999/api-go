package bootstrap

import (
	"context"
	"strings"
	"time"

	i18n "api/common/i18n"
	"api/internal/bootstrap/configload"
	"api/internal/bootstrap/hotreload"
	"api/internal/infra/loggerx"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/core/logx"
)

// ReloadConfig 手动触发一次配置重载，供 handler/logic 通过接口调用。
func (a *App) ReloadConfig(ctx context.Context, source string) error {
	_, err := a.reloadConfigFile(ctx, source, a.boundConfigFile())
	return errors.Tag(err)
}

// startConfigHotReload 在启用时启动后台配置轮询协程。
func (a *App) startConfigHotReload() {
	if a == nil || a.ServiceContext == nil {
		return
	}
	cfg := a.ServiceContext.CurrentConfig()
	interval := hotreload.CheckInterval(cfg.HotReload.CheckIntervalSeconds)
	configFile := a.boundConfigFile()
	a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
		status.Enabled = cfg.HotReload.Enabled
		status.Watching = false
		status.ConfigFile = configFile
		status.CheckIntervalSeconds = int(interval / time.Second)
		status.ConfigVersion = a.ServiceContext.CurrentVersion()
		status.ConfigSummary = hotreload.Summary(cfg)
		if status.LastStatus == "" {
			status.LastStatus = "idle"
			status.LastMessage = "热加载监听尚未启动"
			status.LastMessageKey = i18n.MsgKeyHotReloadWatcherNotStarted
		}
		return status
	})
	if configFile == "" || !cfg.HotReload.Enabled {
		return
	}
	if !a.hotReload.StartWatcher(func(ctx context.Context) {
		a.watchConfigFile(ctx, configFile)
	}) {
		return
	}
	loggerx.Infow(context.Background(), "配置 热加载已启用",
		logx.Field("file", configFile),
		logx.Field(loggerx.FieldIntervalSeconds, int(interval/time.Second)),
	)
}

// stopConfigHotReload 停止配置热加载后台协程。
func (a *App) stopConfigHotReload() {
	if a == nil {
		return
	}
	a.hotReload.StopWatcher()
}

// isConfigHotReloadRunning 返回当前是否已有热加载 watcher 在运行。
func (a *App) isConfigHotReloadRunning() bool {
	if a == nil {
		return false
	}
	return a.hotReload.WatcherRunning()
}

// watchConfigFile 轮询配置文件指纹，检测到变化后重新解析并刷新配置快照。
func (a *App) watchConfigFile(ctx context.Context, configFile string) {
	interval := hotreload.CheckInterval(a.ServiceContext.CurrentConfig().HotReload.CheckIntervalSeconds)
	lastFingerprint, err := configload.BundleFingerprint(configFile)
	if err != nil {
		a.markHotReloadFailure(i18n.MsgKeyHotReloadFingerprintInitFailed, "初始化配置文件指纹失败", err, "", "startup", "fingerprint", configFile)
		a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
			status.Enabled = a.ServiceContext.CurrentConfig().HotReload.Enabled
			status.Watching = false
			status.ConfigFile = configFile
			status.CheckIntervalSeconds = int(interval / time.Second)
			return status
		})
		return
	}
	a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
		status.Enabled = true
		status.Watching = true
		status.ConfigFile = configFile
		status.CheckIntervalSeconds = int(interval / time.Second)
		status.ConfigVersion = a.ServiceContext.CurrentVersion()
		status.ConfigSummary = hotreload.Summary(a.ServiceContext.CurrentConfig())
		status.LastTriggerSource = "startup"
		if status.LastStatus == "" || status.LastStatus == "idle" {
			status.LastStatus = "idle"
			status.LastMessage = "热加载监听运行中"
			status.LastMessageKey = i18n.MsgKeyHotReloadWatcherRunning
		}
		return status
	})
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
				status.Watching = false
				if status.LastMessage == "" {
					status.LastMessage = "热加载监听已停止"
					status.LastMessageKey = i18n.MsgKeyHotReloadWatcherStopped
				}
				return status
			})
			return
		case <-timer.C:
			now := time.Now()
			a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
				status.LastCheckedAt = now
				status.CheckIntervalSeconds = int(hotreload.CheckInterval(a.ServiceContext.CurrentConfig().HotReload.CheckIntervalSeconds) / time.Second)
				return status
			})
			currentFingerprint, statErr := configload.BundleFingerprint(configFile)
			if statErr != nil {
				a.markHotReloadFailure(i18n.MsgKeyHotReloadFileStatusReadFailed, "读取配置文件状态失败", statErr, "", "watcher", "fingerprint", configFile)
				timer.Reset(hotreload.CheckInterval(a.ServiceContext.CurrentConfig().HotReload.CheckIntervalSeconds))
				continue
			}
			if currentFingerprint != lastFingerprint {
				if _, reloadErr := a.reloadConfigFile(ctx, "watcher", configFile); reloadErr == nil {
					lastFingerprint = currentFingerprint
				}
			}
			if !a.ServiceContext.CurrentConfig().HotReload.Enabled {
				a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
					status.Enabled = false
					status.Watching = false
					status.LastStatus = "idle"
					status.LastMessage = "热加载监听已关闭"
					status.LastMessageKey = i18n.MsgKeyHotReloadWatcherClosed
					return status
				})
				return
			}
			timer.Reset(hotreload.CheckInterval(a.ServiceContext.CurrentConfig().HotReload.CheckIntervalSeconds))
		}
	}
}

// reloadConfigFile 串行执行一次配置文件重载，供 watcher 和手动接口共用。
func (a *App) reloadConfigFile(ctx context.Context, source string, configFile string) (string, error) {
	if a == nil || a.ServiceContext == nil {
		return "", errors.Errorf("应用实例为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	configFile = strings.TrimSpace(configFile)
	if configFile == "" {
		notBoundErr := errors.Errorf("未绑定配置文件路径")
		a.markHotReloadFailure(i18n.MsgKeyHotReloadNotBound, "配置热加载未绑定文件", notBoundErr, "", source, "not_bound", configFile)
		return "", notBoundErr
	}
	a.hotReload.LockExec()
	defer a.hotReload.UnlockExec()
	select {
	case <-ctx.Done():
		cancelErr := errors.Tag(ctx.Err())
		a.markHotReloadFailure(i18n.MsgKeyHotReloadCancelled, "配置热加载已取消", cancelErr, "", source, "cancelled", configFile)
		return "", cancelErr
	default:
	}

	beforeCfg := a.ServiceContext.CurrentConfig()
	previousVersion := a.ServiceContext.CurrentVersion()
	currentFingerprint, err := configload.BundleFingerprint(configFile)
	if err != nil {
		a.markHotReloadFailure(i18n.MsgKeyHotReloadFingerprintReadFailed, "读取配置文件指纹失败", err, "", source, "fingerprint", configFile)
		return "", errors.Tag(err)
	}
	cfg, version, err := LoadConfig(configFile)
	if err != nil {
		a.markHotReloadFailure(i18n.MsgKeyHotReloadFailed, "配置热加载失败", err, currentFingerprint, source, "load", configFile)
		return "", errors.Tag(err)
	}
	if previousVersion != "" && version == previousVersion {
		a.markHotReloadUnchanged(configFile, source, version)
		return currentFingerprint, nil
	}
	restartRequired, restartReason := configload.DetectReloadRestartImpact(beforeCfg, cfg)
	effectiveCfg := cfg
	if restartRequired {
		effectiveCfg = configload.BuildReloadEffectiveConfig(beforeCfg, cfg)
	}
	publishRuntimeConfig(effectiveCfg)
	a.ServiceContext.UpdateConfig(effectiveCfg)
	a.ServiceContext.UpdateVersion(version)
	a.updateRuntimeAlertConfig(effectiveCfg)
	now := time.Now()
	message := "配置热加载成功"
	messageKey := i18n.MsgKeyHotReloadSuccess
	if restartRequired {
		message = "配置热加载成功，部分启动期配置需重启后生效"
		messageKey = i18n.MsgKeyHotReloadSuccessRestart
	}
	a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
		status.Enabled = effectiveCfg.HotReload.Enabled
		status.ConfigFile = configFile
		status.CheckIntervalSeconds = int(hotreload.CheckInterval(effectiveCfg.HotReload.CheckIntervalSeconds) / time.Second)
		status.ConfigVersion = version
		status.ConfigSummary = hotreload.Summary(effectiveCfg)
		status.RestartRequired = restartRequired
		status.RestartReason = restartReason
		status.LastStatus = "success"
		status.LastMessage = message
		status.LastMessageKey = messageKey
		status.LastTriggerSource = hotreload.Source(source)
		status.LastFailureCategory = ""
		status.LastReloadAt = now
		status.LastSuccessAt = now
		status.ReloadCount++
		return status
	})
	a.hotReload.ResetFailureLog()
	loggerx.Infow(ctx, "配置 热加载成功",
		logx.Field("file", configFile),
		logx.Field("from_version", previousVersion),
		logx.Field("to_version", version),
		logx.Field("restart_required", restartRequired),
		logx.Field("restart_reason", restartReason),
	)
	if effectiveCfg.HotReload.Enabled && !a.isConfigHotReloadRunning() {
		a.startConfigHotReload()
	}
	if !effectiveCfg.HotReload.Enabled && hotreload.Source(source) != "watcher" {
		a.stopConfigHotReload()
	}
	return currentFingerprint, nil
}

// markHotReloadUnchanged 记录一次无配置变更的热加载检查，不刷新运行配置快照。
func (a *App) markHotReloadUnchanged(configFile, source, version string) {
	if a == nil || a.ServiceContext == nil {
		return
	}
	now := time.Now()
	a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
		status.ConfigFile = strings.TrimSpace(configFile)
		status.ConfigVersion = strings.TrimSpace(version)
		status.ConfigSummary = hotreload.Summary(a.ServiceContext.CurrentConfig())
		status.LastStatus = "success"
		status.LastMessage = "配置无变化"
		status.LastMessageKey = i18n.MsgKeyHotReloadUnchanged
		status.LastTriggerSource = hotreload.Source(source)
		status.LastFailureCategory = ""
		status.LastCheckedAt = now
		return status
	})
}

// boundConfigFile 返回当前 App 绑定的配置文件路径。
func (a *App) boundConfigFile() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.ConfigFile)
}

// refreshHotReloadStatus 在当前状态基础上执行原子更新。
func (a *App) refreshHotReloadStatus(mutator func(svc.HotReloadStatus) svc.HotReloadStatus) {
	if a == nil || a.ServiceContext == nil || mutator == nil {
		return
	}
	a.hotReload.UpdateStatus(a.ServiceContext, mutator)
}

// markHotReloadFailure 记录最近一次热加载失败状态，并对重复错误限频。
func (a *App) markHotReloadFailure(messageKey, message string, err error, fingerprint, source, category, configFile string) {
	if a == nil {
		return
	}
	now := time.Now()
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
		status.LastStatus = "failed"
		status.LastMessageKey = strings.TrimSpace(messageKey)
		if status.LastMessageKey == "" {
			status.LastMessageKey = i18n.MsgKeyHotReloadFailed
		}
		status.LastMessage = strings.TrimSpace(message)
		status.LastReloadAt = now
		status.LastFailureAt = now
		status.LastTriggerSource = hotreload.Source(source)
		status.LastFailureCategory = hotreload.FailureCategory(category)
		if fingerprint != "" {
			status.ConfigVersion = fingerprint
		}
		return status
	})
	errorKey := message + "|" + errText + "|" + source + "|" + category
	if a.hotReload.SuppressFailure(errorKey, now, 30*time.Second) {
		a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
			status.SuppressedFailureCount++
			return status
		})
		return
	}
	loggerx.ErrorTextw(context.Background(), "配置 热加载失败", errText,
		logx.Field("file", configFile),
		logx.Field("detail", message),
		logx.Field("version", fingerprint),
		logx.Field("source", hotreload.Source(source)),
		logx.Field("category", hotreload.FailureCategory(category)),
	)
	a.notifyConfigReloadFailure(message, err, source, category, configFile)
}
