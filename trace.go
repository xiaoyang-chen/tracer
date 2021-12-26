package tracer

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/astaxie/beego/logs"
	"github.com/opentracing/opentracing-go"
	opentracingLog "github.com/opentracing/opentracing-go/log"
	"github.com/uber/jaeger-client-go"
	jaegerCfg "github.com/uber/jaeger-client-go/config"
	"github.com/valyala/fasthttp"
	chenxyUtils "github.com/xiaoyang-chen/utils-golang"
)

const otMwJaegerDebugHeader = "ot-mw-debug-id"
const otMwJaegerBaggageHeader = "ot-mw-baggage"

// OtMwTraceContextHeaderName trace的http header key名称, jaeger解析前会将其转换成小写, 所以不能存在大写字母, 设置跨域header需要, 要开放出去
const OtMwTraceContextHeaderName = "ot-mw-trace-id"
const otMwTraceBaggageHeaderPrefix = "ot-mw-ctx-"

const httpMiddleWareComponentName = "ot-mw-tracer"

const logFieldKeyHttpStatusCode = "http.status_code"

// codecFormat for inject and extract to/from carrier, refer to opentracing.HTTPHeaders
type codecFormat int

const (
	fasthttpHeadersCodecFormat codecFormat = iota
)

type contextKey struct{}

var activeSpanKey = contextKey{}

// check for fasthttpHeadersCodecFormat's carrier
var _ opentracing.TextMapWriter = &fasthttp.RequestHeader{}
var _ opentracing.TextMapReader = &fasthttpRespHeaderCarrier{}

var logFieldHttpMiddleWareComponentName = opentracingLog.String(
	"component", httpMiddleWareComponentName,
)

var pJaegerHeaderConfig = &jaeger.HeadersConfig{
	JaegerDebugHeader:        otMwJaegerDebugHeader,
	JaegerBaggageHeader:      otMwJaegerBaggageHeader,
	TraceContextHeaderName:   OtMwTraceContextHeaderName,
	TraceBaggageHeaderPrefix: otMwTraceBaggageHeaderPrefix,
}
var pJaegerNullMetrics = jaeger.NewNullMetrics()
var pJaegerHttpHeaderPropagator = jaeger.NewHTTPHeaderPropagator(pJaegerHeaderConfig, *pJaegerNullMetrics)
var pJaegerFasthttpHeaderPropagator = jaeger.NewHTTPHeaderPropagator(pJaegerHeaderConfig, *pJaegerNullMetrics)

var noopTracerImpl = &tracerImpl{
	tracer: defaultNoopTracer,
	closer: defaultNoopCloser,
}

type Tracer interface {
	// Close 释放tracer占用的资源
	Close() (err error)
	// StartSpan 生成一个操作名称为opName的起始span(父span)
	StartSpan(opName string) (span opentracing.Span)
	// ChildSpanFromContext 根据ctx里的span信息生成一个操作名称为opName的子span, 如果ctx没有span信息, 将生成一个操作名称为opName的起始span(父span)
	ChildSpanFromContext(opName string, ctx context.Context) (
		child opentracing.Span,
	)
	// FollowerSpanFromContext 根据ctx里的span信息生成一个操作名称为opName的跟随span, 如果ctx没有span信息, 将生成一个操作名称为opName的起始span(父span)
	FollowerSpanFromContext(opName string, ctx context.Context) (
		follower opentracing.Span,
	)
	// ChildSpanFromParent 根据父span生成一个操作名称为opName的子span, 如果parent为nil, 将生成一个操作名称为opName的起始span(父span)
	ChildSpanFromParent(opName string, parent opentracing.Span) (
		child opentracing.Span,
	)
	// FollowerSpanFromParent 根据父span生成一个操作名称为opName的跟随span, 如果parent为nil, 将生成一个操作名称为opName的起始span(父span)
	FollowerSpanFromParent(opName string, parent opentracing.Span) (
		follower opentracing.Span,
	)
	// ChildSpanFromHttpHeader 根据http头里的span信息生成一个操作名称为opName的子span, 如果http头没有span信息, 将生成一个操作名称为opName的起始span(父span)
	ChildSpanFromHttpHeader(opName string, header http.Header) (
		child opentracing.Span,
	)
	// FollowerSpanFromHttpHeader 根据http头里的span信息生成一个操作名称为opName的跟随span, 如果http头没有span信息, 将生成一个操作名称为opName的起始span(父span)
	FollowerSpanFromHttpHeader(opName string, header http.Header) (
		follower opentracing.Span,
	)
	// ChildSpanFromFasthttpHeader 根据 fasthttp.ResponseHeader 头里的span信息生成一个操作名称为opName的子span, 如果 fasthttp.ResponseHeader 头没有span信息, 将生成一个操作名称为opName的起始span(父span)
	ChildSpanFromFasthttpHeader(
		opName string, header *fasthttp.ResponseHeader,
	) (child opentracing.Span)
	// FollowerSpanFromFasthttpHeader 根据 fasthttp.ResponseHeader 头里的span信息生成一个操作名称为opName的跟随span, 如果 fasthttp.ResponseHeader 头没有span信息, 将生成一个操作名称为opName的起始span(父span)
	FollowerSpanFromFasthttpHeader(
		opName string, header *fasthttp.ResponseHeader,
	) (follower opentracing.Span)
	// LogCodeAndMsgToSpan 已log的形式记录code和msg到span
	LogCodeAndMsgToSpan(span opentracing.Span, code int, msg string)
	// ContextWithSpan 将span注入ctx生成新的ctx, ctxWithChild携带新生成的span信息, 当span为nil时返回传入的ctx
	ContextWithSpan(ctx context.Context, span opentracing.Span) (
		ctxWithSpan context.Context,
	)
	// CtxWithSpanCtxFromHttpHeader 从 http.Header 中获取 SpanContext 信息, 并将之注入到ctx中, 生成新的ctx, 当未获取到 SpanContext 信息时返回传入的ctx
	CtxWithSpanCtxFromHttpHeader(ctx context.Context, header http.Header) (
		newCtx context.Context,
	)
	// CtxWithSpanCtxFromFasthttpHeader 从 *fasthttp.ResponseHeader 中获取 SpanContext 信息, 并将之注入到ctx中, 生成新的ctx, 当未获取到 SpanContext 信息时返回传入的ctx
	CtxWithSpanCtxFromFasthttpHeader(
		ctx context.Context, header *fasthttp.ResponseHeader,
	) (newCtx context.Context)
	// Inject2HttpHeader 将span信息打进http头里, 便于在不同服务间传递span信息
	Inject2HttpHeader(span opentracing.Span, header http.Header) (err error)
	// Inject2FasthttpHeader 将span信息打进fasthttp头里, 便于在不同服务间传递span信息
	Inject2FasthttpHeader(
		span opentracing.Span, header *fasthttp.RequestHeader,
	) (err error)
	// Inject2HttpHeaderByCtx 将ctx里的span信息打进http头里, 便于在不同服务间传递span信息
	Inject2HttpHeaderByCtx(ctx context.Context, header http.Header) (err error)
	// Inject2FasthttpHeaderByCtx 将ctx里的span信息打进fasthttp头里, 便于在不同服务间传递span信息
	Inject2FasthttpHeaderByCtx(
		ctx context.Context, header *fasthttp.RequestHeader,
	) (err error)
	// HttpMiddleWare 返回带有该tracer信息的http.Handler, 返回的http.Handler将根据http的request的header里的span信息生成一个子span, 并将其注入的request.context中(如果http的request的header中没有span信息, 将生成一个父span, 并将其信息注入request.context中)
	HttpMiddleWare(handler http.Handler) (traceHandler http.Handler)
	// GetFasthttp 通过fasthttp发起get请求
	GetFasthttp(
		ctx context.Context, url string,
		mapHeader, mapCookie map[string]string, cbs ...FasthttpRespCallback,
	) (newCtx context.Context, respBody []byte, err error)
	// PostJsonFasthttp 通过fasthttp发起post请求, data为可json序列化的结构数据
	PostJsonFasthttp(
		ctx context.Context, url string, data interface{},
		mapHeader, mapCookie map[string]string, cbs ...FasthttpRespCallback,
	) (newCtx context.Context, respBody []byte, err error)
	// PutJsonFasthttp 通过fasthttp发起put请求, data为可json序列化的结构数据
	PutJsonFasthttp(
		ctx context.Context, url string, data interface{},
		mapHeader, mapCookie map[string]string, cbs ...FasthttpRespCallback,
	) (newCtx context.Context, respBody []byte, err error)
	// DeleteJsonFasthttp 通过fasthttp发起delete请求, data为可json序列化的结构数据
	DeleteJsonFasthttp(
		ctx context.Context, url string, data interface{},
		mapHeader, mapCookie map[string]string, cbs ...FasthttpRespCallback,
	) (newCtx context.Context, respBody []byte, err error)
}

type tracerImpl struct {
	tracer opentracing.Tracer
	closer io.Closer
}

func InitEmptyTracer() Tracer { return noopTracerImpl }

// NewTracerBySrvNameAndTracerSrvHost 根据服务名称和tracer服务地址创建Tracer实例, 目前内部的实现方式为全追踪模式并通过http直连jaeger服务端上报追踪信息且设置为opentracing中的全局唯一tracer, 内置的log为beego默认的BeeLogger; 返回的tracer可在服务内并发使用, 在程序退出前通过调用tracer.Close()释放tracer占用的资源; example: NewTracerBySrvNameAndTracerSrvHost("tracer-self", "http://127.0.0.1:14268")
func NewTracerBySrvNameAndTracerSrvHost(srvName, tracerSrvHost string) (
	tracer Tracer, err error,
) {

	var opentracingTracer opentracing.Tracer
	var closer io.Closer
	if opentracingTracer, closer, err = newTracerInConstSampleWithBeegoLogByDR(
		srvName, tracerSrvHost,
	); err != nil {
		return
	}
	// 取消设置为全局, 防止误用
	// opentracing.SetGlobalTracer(opentracingTracer)
	tracer = &tracerImpl{
		tracer: opentracingTracer,
		closer: closer,
	}
	return
}

func (ti *tracerImpl) Close() (err error) {

	err = ti.closer.Close()
	return
}

func (ti *tracerImpl) StartSpan(opName string) (span opentracing.Span) {

	span = ti.tracer.StartSpan(opName)
	return
}

func (ti *tracerImpl) ChildSpanFromContext(
	opName string, ctx context.Context,
) (child opentracing.Span) {

	if span, spanCtx := ti.spanInfoFromContext(ctx); span != nil {
		child = ti.ChildSpanFromParent(opName, span)
	} else if spanCtx != nil {
		child = ti.tracer.StartSpan(opName, opentracing.ChildOf(spanCtx))
	} else {
		child = ti.StartSpan(opName)
	}
	return
}

func (ti *tracerImpl) FollowerSpanFromContext(
	opName string, ctx context.Context,
) (follower opentracing.Span) {

	if span, spanCtx := ti.spanInfoFromContext(ctx); span != nil {
		follower = ti.FollowerSpanFromParent(opName, span)
	} else if spanCtx != nil {
		follower = ti.tracer.StartSpan(opName, opentracing.FollowsFrom(spanCtx))
	} else {
		follower = ti.StartSpan(opName)
	}
	return
}

func (ti *tracerImpl) ChildSpanFromParent(
	opName string, parent opentracing.Span,
) (child opentracing.Span) {

	if parent == nil {
		child = ti.StartSpan(opName)
	} else {
		child = ti.tracer.StartSpan(
			opName, opentracing.ChildOf(parent.Context()),
		)
	}
	return
}

func (ti *tracerImpl) FollowerSpanFromParent(
	opName string, parent opentracing.Span,
) (follower opentracing.Span) {

	if parent == nil {
		follower = ti.StartSpan(opName)
	} else {
		follower = ti.tracer.StartSpan(
			opName, opentracing.FollowsFrom(parent.Context()),
		)
	}
	return
}

func (ti *tracerImpl) ChildSpanFromHttpHeader(
	opName string, header http.Header,
) (child opentracing.Span) {

	child = ti.getSpanFromHttpHeader(opName, header, opentracing.ChildOfRef)
	return
}

func (ti *tracerImpl) FollowerSpanFromHttpHeader(
	opName string, header http.Header,
) (follower opentracing.Span) {

	follower = ti.getSpanFromHttpHeader(
		opName, header, opentracing.FollowsFromRef,
	)
	return
}

func (ti *tracerImpl) ChildSpanFromFasthttpHeader(
	opName string, header *fasthttp.ResponseHeader,
) (child opentracing.Span) {

	child = ti.getSpanFromFasthttpHeader(opName, header, opentracing.ChildOfRef)
	return
}

func (ti *tracerImpl) FollowerSpanFromFasthttpHeader(
	opName string, header *fasthttp.ResponseHeader,
) (follower opentracing.Span) {

	follower = ti.getSpanFromFasthttpHeader(
		opName, header, opentracing.FollowsFromRef,
	)
	return
}

func (ti *tracerImpl) LogCodeAndMsgToSpan(
	span opentracing.Span, code int, msg string,
) {

	if span == nil {
		return
	}

	span.LogFields(
		opentracingLog.Int("code", code),
		opentracingLog.String("msg", msg),
	)
}

func (ti *tracerImpl) ContextWithSpan(
	ctx context.Context, span opentracing.Span,
) (ctxWithSpan context.Context) {

	if ctx == nil || span == nil {
		ctxWithSpan = ctx
	} else {
		ctxWithSpan = context.WithValue(ctx, activeSpanKey, span)
	}
	return
}

func (ti *tracerImpl) CtxWithSpanCtxFromHttpHeader(
	ctx context.Context, header http.Header,
) (newCtx context.Context) {

	if ctx == nil {
		return
	}

	if spanCtx, _ := ti.extractFromHttpHeader(header); spanCtx == nil {
		newCtx = ctx
	} else {
		newCtx = context.WithValue(ctx, activeSpanKey, spanCtx)
	}
	return
}

func (ti *tracerImpl) CtxWithSpanCtxFromFasthttpHeader(
	ctx context.Context, header *fasthttp.ResponseHeader,
) (newCtx context.Context) {

	if ctx == nil {
		return
	}

	if spanCtx, _ := ti.extractFromFasthttpHeader(header); spanCtx == nil {
		newCtx = ctx
	} else {
		newCtx = context.WithValue(ctx, activeSpanKey, spanCtx)
	}
	return
}

func (ti *tracerImpl) Inject2HttpHeader(
	span opentracing.Span, header http.Header,
) (err error) {

	if span != nil {
		err = ti.injectSpanCtx2HttpHeader(span.Context(), header)
	}
	return
}

func (ti *tracerImpl) Inject2FasthttpHeader(
	span opentracing.Span, header *fasthttp.RequestHeader,
) (err error) {

	if span != nil {
		err = ti.injectSpanCtx2FasthttpHeader(span.Context(), header)
	}
	return
}

func (ti *tracerImpl) Inject2HttpHeaderByCtx(
	ctx context.Context, header http.Header,
) (err error) {

	if span, spanCtx := ti.spanInfoFromContext(ctx); span != nil {
		err = ti.injectSpanCtx2HttpHeader(span.Context(), header)
	} else if spanCtx != nil {
		err = ti.injectSpanCtx2HttpHeader(spanCtx, header)
	}
	return
}

func (ti *tracerImpl) Inject2FasthttpHeaderByCtx(
	ctx context.Context, header *fasthttp.RequestHeader,
) (err error) {

	if span, spanCtx := ti.spanInfoFromContext(ctx); span != nil {
		err = ti.injectSpanCtx2FasthttpHeader(span.Context(), header)
	} else if spanCtx != nil {
		err = ti.injectSpanCtx2FasthttpHeader(spanCtx, header)
	}
	return
}

func (ti *tracerImpl) HttpMiddleWare(handler http.Handler) (
	traceHandler http.Handler,
) {

	if ti == noopTracerImpl {
		traceHandler = handler
		return
	}

	traceHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var child = ti.ChildSpanFromHttpHeader(
			getOperationNameFromHttpRequest(r), r.Header,
		)

		r = r.WithContext(ti.ContextWithSpan(r.Context(), child))
		var sct = &statusCodeTracker{ResponseWriter: w}
		handler.ServeHTTP(sct, r)

		child.LogFields(
			logFieldHttpMiddleWareComponentName,
			opentracingLog.Int(logFieldKeyHttpStatusCode, sct.statusCode),
		)
		child.Finish()
	})
	return
}

// spanInfoFromContext 从ctx中获取span或spanCtx, 优先获取span, 当没有获取到span时再获取spanCtx, 当从ctx中都获取不到时返回(nil, nil)
func (ti *tracerImpl) spanInfoFromContext(ctx context.Context) (
	span opentracing.Span, spanCtx opentracing.SpanContext,
) {

	if ctx == nil {
		return
	}

	switch val := ctx.Value(activeSpanKey); v := val.(type) {
	case opentracing.Span:
		span = v
	case opentracing.SpanContext:
		spanCtx = v
	}
	return
}

// getSpanFromHttpHeader 从 http.Header 中获取span, 并创建对应类型的子span, 当从 http.Header 中没获取到时将根据opName创建一个起始span(父span)
func (ti *tracerImpl) getSpanFromHttpHeader(
	opName string, header http.Header, refType opentracing.SpanReferenceType,
) (span opentracing.Span) {

	if spanCtx, _ := ti.extractFromHttpHeader(header); spanCtx == nil {
		span = ti.tracer.StartSpan(opName)
	} else {
		if refType == opentracing.ChildOfRef {
			span = ti.tracer.StartSpan(opName, opentracing.ChildOf(spanCtx))
		} else {
			span = ti.tracer.StartSpan(opName, opentracing.FollowsFrom(spanCtx))
		}
	}
	return
}

func (ti *tracerImpl) extractFromHttpHeader(header http.Header) (
	spanCtx opentracing.SpanContext, err error,
) {

	spanCtx, err = ti.tracer.Extract(
		opentracing.HTTPHeaders,
		opentracing.HTTPHeadersCarrier(header),
	)
	return
}

// getSpanFromFasthttpHeader 从 fasthttp.ResponseHeader 中获取span, 并创建对应类型的子span, 当从 fasthttp.ResponseHeader 中没获取到时将根据opName创建一个起始span(父span)
func (ti *tracerImpl) getSpanFromFasthttpHeader(
	opName string, header *fasthttp.ResponseHeader,
	refType opentracing.SpanReferenceType,
) (span opentracing.Span) {

	if spanCtx, _ := ti.extractFromFasthttpHeader(header); spanCtx == nil {
		span = ti.tracer.StartSpan(opName)
	} else {
		if refType == opentracing.ChildOfRef {
			span = ti.tracer.StartSpan(opName, opentracing.ChildOf(spanCtx))
		} else {
			span = ti.tracer.StartSpan(opName, opentracing.FollowsFrom(spanCtx))
		}
	}
	return
}

func (ti *tracerImpl) extractFromFasthttpHeader(
	header *fasthttp.ResponseHeader,
) (spanCtx opentracing.SpanContext, err error) {

	spanCtx, err = ti.tracer.Extract(
		fasthttpHeadersCodecFormat,
		(*fasthttpRespHeaderCarrier)(header),
	)
	return
}

func (ti *tracerImpl) injectSpanCtx2HttpHeader(
	spanCtx opentracing.SpanContext, header http.Header,
) (err error) {

	err = ti.tracer.Inject(
		spanCtx, opentracing.HTTPHeaders,
		opentracing.HTTPHeadersCarrier(header),
	)
	return
}

func (ti *tracerImpl) injectSpanCtx2FasthttpHeader(
	spanCtx opentracing.SpanContext, header *fasthttp.RequestHeader,
) (err error) {

	err = ti.tracer.Inject(spanCtx, fasthttpHeadersCodecFormat, header)
	return
}

func getOperationNameFromHttpRequest(r *http.Request) (opName string) {

	var opNameBuild strings.Builder
	// 6 == len("HTTP ") + " "
	opNameBuild.Grow(6 + len(r.Method) + len(r.URL.Path))
	opNameBuild.WriteString("HTTP ")
	opNameBuild.WriteString(r.Method)
	opNameBuild.WriteByte(' ')
	opNameBuild.WriteString(r.URL.Path)
	opName = opNameBuild.String()
	return
}

func newTracerInConstSampleWithBeegoLogByDR(srvName, tracerSrvHost string) (
	tracer opentracing.Tracer, closer io.Closer, err error,
) {

	tracer, closer, err = jaegerCfg.Configuration{
		ServiceName: srvName,
		// Constant (sampler.type=const) sampler always makes the same decision for all traces. It either samples all traces (sampler.param=1) or none of them (sampler.param=0).
		Sampler: &jaegerCfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegerCfg.ReporterConfig{
			// BufferFlushInterval: 1 * time.Second,
			// LogSpans:            true,
			CollectorEndpoint: tracerSrvHost + "/api/traces",
		},
	}.NewTracer(
		jaegerCfg.Logger(newJaegerLogByBeegoLog()),
		jaegerCfg.Injector(
			opentracing.HTTPHeaders, pJaegerHttpHeaderPropagator,
		),
		jaegerCfg.Extractor(
			opentracing.HTTPHeaders, pJaegerHttpHeaderPropagator,
		),
		jaegerCfg.Injector(
			fasthttpHeadersCodecFormat, pJaegerFasthttpHeaderPropagator,
		),
		jaegerCfg.Extractor(
			fasthttpHeadersCodecFormat, pJaegerFasthttpHeaderPropagator,
		),
	)
	return
}

type beegoLog2JaegerLog struct {
	pBeegoLog *logs.BeeLogger
}

func newJaegerLogByBeegoLog() (jLog jaeger.Logger) {
	return &beegoLog2JaegerLog{pBeegoLog: logs.GetBeeLogger()}
}

func (b2j *beegoLog2JaegerLog) Error(msg string) { b2j.pBeegoLog.Warn(msg) }

func (b2j *beegoLog2JaegerLog) Infof(msg string, args ...interface{}) {
	b2j.pBeegoLog.Info(msg, args...)
}

type statusCodeTracker struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusCodeTracker) WriteHeader(statusCode int) {

	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

type fasthttpRespHeaderCarrier fasthttp.ResponseHeader

func (frhc *fasthttpRespHeaderCarrier) ForeachKey(
	handler func(key, val string) error,
) (err error) {

	(*fasthttp.ResponseHeader)(frhc).VisitAll(func(key, value []byte) {
		err = handler(chenxyUtils.Bytes2Str(key), chenxyUtils.Bytes2Str(value))
	})
	return
}
