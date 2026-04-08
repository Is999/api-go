package tracing

import (
	"context"
	"net/url"
	"strings"

	"gozero_api/internal/config"

	"github.com/Is999/go-utils/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Setup 初始化 OpenTelemetry provider。
func Setup(ctx context.Context, cfg config.ObservabilityConfig) (func(context.Context) error, error) {
	serviceName := strings.TrimSpace(cfg.ServiceName)
	if serviceName == "" {
		serviceName = "gozero_api"
	}
	environment := strings.TrimSpace(cfg.Environment)
	if environment == "" {
		environment = "unknown"
	}

	resource, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("deployment.environment", environment),
		),
	)
	if err != nil {
		return nil, errors.Wrap(err, "构建 OTEL 资源失败")
	}

	sampleRatio := cfg.SampleRatio
	if sampleRatio <= 0 || sampleRatio > 1 {
		sampleRatio = 1
	}
	if !cfg.TraceEnabled {
		sampleRatio = 0
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(sampleRatio)),
		sdktrace.WithResource(resource),
	}
	if cfg.OTLPEndpoint != "" {
		protocol := normalizeOTLPProtocol(cfg.OTLPProtocol)
		switch protocol {
		case "grpc":
			endpoint, _ := normalizeOTLPEndpoint(cfg.OTLPEndpoint)
			exporterOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
			if cfg.OTLPInsecure {
				exporterOpts = append(exporterOpts, otlptracegrpc.WithInsecure())
			}
			exporter, err := otlptracegrpc.New(ctx, exporterOpts...)
			if err != nil {
				return nil, errors.Wrap(err, "初始化 OTLP 导出器失败")
			}
			options = append(options, sdktrace.WithBatcher(exporter))
		case "http":
			endpoint, urlPath := normalizeOTLPEndpoint(cfg.OTLPEndpoint)
			if urlPath == "" {
				urlPath = "/v1/traces"
			}
			exporterOpts := []otlptracehttp.Option{
				otlptracehttp.WithEndpoint(endpoint),
				otlptracehttp.WithURLPath(urlPath),
			}
			if cfg.OTLPInsecure {
				exporterOpts = append(exporterOpts, otlptracehttp.WithInsecure())
			}
			exporter, err := otlptracehttp.New(ctx, exporterOpts...)
			if err != nil {
				return nil, errors.Wrap(err, "初始化 OTLP 导出器失败")
			}
			options = append(options, sdktrace.WithBatcher(exporter))
		default:
			return nil, errors.Errorf("不支持的 otlp_protocol: %s", strings.TrimSpace(cfg.OTLPProtocol))
		}
	}

	tp := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

// normalizeOTLPProtocol 归一化 OTLP 协议别名。
func normalizeOTLPProtocol(protocol string) string {
	protocol = strings.TrimSpace(strings.ToLower(protocol))
	switch protocol {
	case "", "grpc", "grpc/protobuf":
		return "grpc"
	case "http", "http/protobuf", "http-protobuf", "http/json":
		return "http"
	default:
		return protocol
	}
}

// normalizeOTLPEndpoint 拆分 OTLP endpoint 的 host 和 path。
func normalizeOTLPEndpoint(endpoint string) (string, string) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", ""
	}
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		if err == nil && u.Host != "" {
			path := strings.TrimSpace(u.Path)
			if path == "" || path == "/" {
				path = ""
			}
			return u.Host, path
		}
	}
	if idx := strings.Index(endpoint, "/"); idx > 0 {
		host := strings.TrimSpace(endpoint[:idx])
		path := strings.TrimSpace(endpoint[idx:])
		if path == "" || path == "/" {
			path = ""
		}
		return host, path
	}
	return endpoint, ""
}
