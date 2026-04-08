package types

import "time"

// ConfigReloadStatusResp 表示 config.yaml 热加载运行状态。
type ConfigReloadStatusResp struct {
	Enabled                bool      `json:"enabled"`                // 是否启用热加载
	Watching               bool      `json:"watching"`               // 当前是否正在监听配置文件
	ConfigFile             string    `json:"configFile"`             // 当前监听的配置文件路径
	CheckIntervalSeconds   int       `json:"checkIntervalSeconds"`   // 轮询间隔，单位秒
	ConfigVersion          string    `json:"configVersion"`          // 当前配置版本指纹
	ConfigSummary          string    `json:"configSummary"`          // 当前配置摘要
	RestartRequired        bool      `json:"restartRequired"`        // 是否需要重启才能完全生效
	RestartReason          string    `json:"restartReason"`          // 需要重启的原因摘要
	LastStatus             string    `json:"lastStatus"`             // 最近一次处理结果
	LastMessage            string    `json:"lastMessage"`            // 最近一次处理结果说明
	LastTriggerSource      string    `json:"lastTriggerSource"`      // 最近一次触发来源
	LastFailureCategory    string    `json:"lastFailureCategory"`    // 最近一次失败分类
	LastCheckedAt          time.Time `json:"lastCheckedAt"`          // 最近一次检查时间
	LastReloadAt           time.Time `json:"lastReloadAt"`           // 最近一次重载时间
	LastSuccessAt          time.Time `json:"lastSuccessAt"`          // 最近一次成功时间
	LastFailureAt          time.Time `json:"lastFailureAt"`          // 最近一次失败时间
	ReloadCount            int64     `json:"reloadCount"`            // 累计成功加载次数
	SuppressedFailureCount int64     `json:"suppressedFailureCount"` // 限频压制的重复失败日志次数
}
