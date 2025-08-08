package mserve

//import (
//	"context"
//	"go.opentelemetry.io/contrib/exporters/autoexport"
//	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
//	"go.opentelemetry.io/otel"
//	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
//	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
//
//	"go.opentelemetry.io/otel/propagation"
//	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
//	sdktrace "go.opentelemetry.io/otel/sdk/trace"
//	"net/http"
//)
//
//func initTracer() (*sdktrace.TracerProvider, *sdkmetric.MeterProvider, error) {
//	ctx := context.Background()
//	client := otlptracehttp.NewClient()
//
//	otlpTraceExporter, err := otlptrace.New(ctx, client)
//	if err != nil {
//		return nil, nil, err
//	}
//	metric, err := autoexport.NewMetricReader(ctx)
//	if err != nil {
//		return nil, nil, err
//	}
//
//	batchSpanProcessor := sdktrace.NewBatchSpanProcessor(otlpTraceExporter)
//
//	tracerProvider := sdktrace.NewTracerProvider(
//		sdktrace.WithSampler(sdktrace.AlwaysSample()),
//		sdktrace.WithSpanProcessor(batchSpanProcessor),
//	)
//
//	otel.SetTracerProvider(tracerProvider)
//	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
//		propagation.TraceContext{}, propagation.Baggage{}))
//	http.DefaultClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
//
//	meterProvider := sdkmetric.NewMeterProvider(
//		sdkmetric.WithReader(metric),
//	)
//	otel.SetMeterProvider(meterProvider)
//	return tracerProvider, meterProvider, nil
//}
