package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/astaxie/beego"
	tracer "github.com/xiaoyang-chen/tracer"
)

const (
	serviceName   = "service-a"
	tracerSrvHost = "http://127.0.0.1:14268"
)

var globalTracer = tracer.InitEmptyTracer()
var globalTracerSwitch = true

func main() {

	// 1. init tracer
	if globalTracerSwitch {
		var err error
		globalTracer, err = tracer.
			NewTracerBySrvNameAndTracerSrvHost(serviceName, tracerSrvHost)
		if err != nil {
			panic(err.Error())
		}
		defer globalTracer.Close()
	}
	// 2. set router
	beego.Router("/ping", &pingController{}, "get:Ping")
	// 3. use the middleware
	beego.RunWithMiddleWares("localhost:8081", globalTracer.HttpMiddleWare)
}

type pingController struct{ baseController }

func (c *pingController) Ping() {

	var ctx = c.Ctx.Request.Context()
	var respBody []byte
	var err error
	ctx, respBody, err = pongService(ctx, 1*time.Second)
	if err != nil {
		respBody = []byte(err.Error())
	}

	c.Success(ctx, respBody)
}

func pongService(ctx context.Context, sleep time.Duration) (
	newCtx context.Context, respBody []byte, err error,
) {

	time.Sleep(sleep)
	if newCtx, respBody, err = globalTracer.GetFasthttp(
		ctx, "http://localhost:8082/pong", nil, nil,
	); err != nil {
		return
	}

	newCtx, err = otherService(newCtx, "chenxy", "tracer", "beego", "example")
	return
}

func otherService(ctx context.Context, args ...interface{}) (
	newCtx context.Context, err error,
) {

	var child = globalTracer.ChildSpanFromContext("otherService", ctx)
	defer child.Finish()
	newCtx = globalTracer.ContextWithSpan(ctx, child)

	fmt.Println("otherService start")
	fmt.Println(args...)
	fmt.Println("otherService end")
	return
}

type baseController struct{ beego.Controller }

func (c *baseController) Success(ctx context.Context, body []byte) {

	globalTracer.Inject2HttpHeaderByCtx(ctx, c.Ctx.ResponseWriter.Header())
	c.Ctx.ResponseWriter.WriteHeader(http.StatusOK)
	c.Ctx.WriteString(fmt.Sprintf("%s -> %s", serviceName, string(body)))
}
