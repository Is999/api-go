package cache

import (
	"context"
	"testing"

	"api/common/runtimecfg"
	appconfig "api/internal/config"
	corelogic "api/internal/logic"
	"api/internal/svc"

	tablecache "github.com/Is999/table-cache"
	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// TestTableCacheManagerRecordsMetrics 验证 API 的真实 table-cache Manager 会记录组件级 Prometheus 指标。
func TestTableCacheManagerRecordsMetrics(t *testing.T) {
	previousRuntime := runtimecfg.Get()
	runtimecfg.Set(appconfig.Config{AppID: "site-a"})
	t.Cleanup(func() { runtimecfg.Restore(previousRuntime) })

	ctx := context.Background()
	registry := prometheus.NewRegistry()
	metrics, err := tablecache.NewPrometheusMetrics(
		tablecache.WithPrometheusRegisterer(registry),
		tablecache.WithPrometheusSubsystem(TableCacheMetricsSubsystem),
	)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	svcCtx := svc.NewServiceContext(appconfig.Config{AppID: "site-a"}, "v1", svc.Dependencies{
		Rds:               client,
		TableCacheMetrics: metrics,
	})
	base := corelogic.NewBaseLogicWithContext(ctx, svcCtx)
	manager, err := TableCacheManager(base)
	if err != nil {
		t.Fatalf("TableCacheManager() error = %v", err)
	}
	key := TableCachePhysicalKey(base, "config_uuid:featureFlag")
	if err = client.HSet(ctx, key, "value", "enabled").Err(); err != nil {
		t.Fatalf("HSet(%s) error = %v", key, err)
	}
	var value map[string]string
	result, err := manager.GetState(ctx, key, &value)
	if err != nil {
		t.Fatalf("GetState(%s) error = %v", key, err)
	}
	if result.State != tablecache.LookupStateHit {
		t.Fatalf("GetState(%s) state = %s, want hit", key, result.State)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("registry.Gather() error = %v", err)
	}
	for _, family := range families {
		if family.GetName() == "tcache_cache_hit_total" {
			return
		}
	}
	t.Fatal("tcache_cache_hit_total metric not found")
}
