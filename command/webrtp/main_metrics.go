package main

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	otextport "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	streamClientsGauge     metric.Int64Gauge
	streamBitrateKbpsGauge metric.Float64Gauge
	streamFramerateGauge   metric.Float64Gauge
)

func MetricsInit(serviceName string) error {
	exporter, err := otextport.New(
		otextport.WithRegisterer(prometheus.DefaultRegisterer),
	)
	if err != nil {
		return err
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)
	otel.SetMeterProvider(provider)

	otelMeter := otel.Meter(serviceName)

	streamClientsGauge, err = otelMeter.Int64Gauge("webrtp_streamer_clients",
		metric.WithDescription("Current number of clients connected to the stream"),
	)
	if err != nil {
		return err
	}

	streamBitrateKbpsGauge, err = otelMeter.Float64Gauge("webrtp_streamer_bitrate_kbps",
		metric.WithDescription("Current bitrate in Kbps"),
	)
	if err != nil {
		return err
	}

	streamFramerateGauge, err = otelMeter.Float64Gauge("webrtp_streamer_framerate",
		metric.WithDescription("Current framerate"),
	)
	if err != nil {
		return err
	}

	return nil
}

func MetricsRoute(app *fiber.App) {
	handler := promhttp.Handler()
	app.Get("/metrics", adaptor.HTTPHandler(handler))
}

func MetricsUpdate(streams []*Stream) {
	if streamClientsGauge == nil {
		return
	}

	ctx := context.Background()

	for _, s := range streams {
		stats := s.Hub.GetStats(s.Name)

		nameAttr := attribute.String("name", s.Name)

		// Clients always shows current value (don't reset to 0 when not ready)
		streamClientsGauge.Record(ctx, int64(stats.ClientCount), metric.WithAttributes(nameAttr))

		// Bitrate and framerate show 0 if stream not ready
		if stats.Ready {
			streamBitrateKbpsGauge.Record(ctx, stats.Bitrate, metric.WithAttributes(nameAttr))
			streamFramerateGauge.Record(ctx, stats.Framerate, metric.WithAttributes(nameAttr))
		} else {
			streamBitrateKbpsGauge.Record(ctx, 0, metric.WithAttributes(nameAttr))
			streamFramerateGauge.Record(ctx, 0, metric.WithAttributes(nameAttr))
		}
	}
}
