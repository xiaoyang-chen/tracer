package tracer

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/valyala/fasthttp"
)

const httpClientTimeOut = 60 * time.Second
const tlsConfigInsecureSkipVerify = true
const defaultUrlLength = 256

var jsonSerializer serializer = jsoniter.ConfigCompatibleWithStandardLibrary

type serializer interface {
	Marshal(v interface{}) ([]byte, error)
	Unmarshal(data []byte, v interface{}) error
}

type FasthttpRespCallback func(ctx context.Context, resp *fasthttp.Response)

// MakeupUrlByHostPathQueryParams 根据给定的host, path, queryParams获取组成的url; 参数示例: host "http://localhost:18200", path: "/path/xxx", queryParams: map[string]string{ "abc": "213123", "def": "213123" }; 返回值示例: "http://localhost:18200/path/xxx?abc=213123&def=213123"
func MakeupUrlByHostPathQueryParams(
	host, path string, queryParams map[string]string,
) (url string) {

	var urlBuild strings.Builder
	if len(queryParams) == 0 {
		urlBuild.Grow(len(host) + len(path))
		urlBuild.WriteString(host)
		urlBuild.WriteString(path)
		url = urlBuild.String()
	} else {
		urlBuild.Grow(defaultUrlLength)
		urlBuild.WriteString(host)
		urlBuild.WriteString(path)
		urlBuild.WriteByte('?')
		for k, v := range queryParams {
			urlBuild.WriteString(k)
			urlBuild.WriteByte('=')
			urlBuild.WriteString(v)
			urlBuild.WriteByte('&')
		}
		url = urlBuild.String()
		url = url[:len(url)-1]
	}
	return
}

func (ti *tracerImpl) GetFasthttp(
	ctx context.Context, url string,
	mapHeader, mapCookie map[string]string, cbs ...FasthttpRespCallback,
) (newCtx context.Context, respBody []byte, err error) {

	newCtx, respBody, err = ti.fasthttpReq(
		ctx, url, http.MethodGet, nil, mapHeader, mapCookie, cbs...,
	)
	return
}

func (ti *tracerImpl) PostJsonFasthttp(
	ctx context.Context, url string, data interface{},
	mapHeader, mapCookie map[string]string, cbs ...FasthttpRespCallback,
) (newCtx context.Context, respBody []byte, err error) {

	newCtx, respBody, err = ti.fasthttpReq(
		ctx, url, http.MethodPost, data, mapHeader, mapCookie, cbs...,
	)
	return
}

func (ti *tracerImpl) DeleteJsonFasthttp(
	ctx context.Context, url string, data interface{},
	mapHeader, mapCookie map[string]string, cbs ...FasthttpRespCallback,
) (newCtx context.Context, respBody []byte, err error) {

	newCtx, respBody, err = ti.fasthttpReq(
		ctx, url, http.MethodDelete, data, mapHeader, mapCookie, cbs...,
	)
	return
}

func (ti *tracerImpl) PutJsonFasthttp(
	ctx context.Context, url string, data interface{},
	mapHeader, mapCookie map[string]string, cbs ...FasthttpRespCallback,
) (newCtx context.Context, respBody []byte, err error) {

	newCtx, respBody, err = ti.fasthttpReq(
		ctx, url, http.MethodPut, data, mapHeader, mapCookie, cbs...,
	)
	return
}

func (ti *tracerImpl) fasthttpReq(
	ctx context.Context, url, method string, jsonData interface{},
	mapHeader, mapCookie map[string]string, cbs ...FasthttpRespCallback,
) (newCtx context.Context, respBody []byte, err error) {

	var childSpan = ti.ChildSpanFromContext(url, ctx)
	defer childSpan.Finish()

	var req = fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	if err = ti.Inject2FasthttpHeader(childSpan, &req.Header); err != nil {
		return
	}
	if jsonData != nil {
		if err = setFasthttpReqBodyByJsonData(req, jsonData); err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
	}
	req.SetRequestURI(url)
	req.Header.SetMethod(method)
	setFasthttpReqHeaderByMap(req, mapHeader)
	setFasthttpReqCookiesByMap(req, mapCookie)

	var resp = fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	if err = sendFasthttpReqWithTimeOut(req, resp); err != nil {
		return
	}
	newCtx = ti.CtxWithSpanCtxFromFasthttpHeader(ctx, &resp.Header)

	applyFasthttpRespCallback(newCtx, resp, cbs...)
	respBody = getFasthttpRespBody(resp)
	return
}

func setFasthttpReqCookiesByMap(
	req *fasthttp.Request, mapCookie map[string]string,
) {

	for k, v := range mapCookie {
		req.Header.SetCookie(k, v)
	}
}

func setFasthttpReqHeaderByMap(
	req *fasthttp.Request, mapHeader map[string]string,
) {

	for k, v := range mapHeader {
		req.Header.Set(k, v)
	}
}

func setFasthttpReqBodyByJsonData(
	req *fasthttp.Request, data interface{},
) (err error) {

	var body []byte
	if body, err = jsonSerializer.Marshal(data); err != nil {
		return
	}
	req.SetBody(body)
	return
}
func sendFasthttpReqWithTimeOut(
	req *fasthttp.Request, resp *fasthttp.Response,
) (err error) {

	err = (&fasthttp.Client{
		TLSConfig: &tls.Config{InsecureSkipVerify: tlsConfigInsecureSkipVerify},
	}).DoTimeout(req, resp, httpClientTimeOut)
	return
}

func applyFasthttpRespCallback(
	ctx context.Context, resp *fasthttp.Response, cbs ...FasthttpRespCallback,
) {

	for _, cb := range cbs {
		cb(ctx, resp)
	}
}

func getFasthttpRespBody(resp *fasthttp.Response) (body []byte) {

	var respBody = resp.Body()
	body = make([]byte, len(respBody))
	copy(body, respBody)
	return
}
