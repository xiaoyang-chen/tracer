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
	serviceName   = "service-b"
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
	beego.Router("/pong", &pongController{}, "get:Pong")
	// 3. use the middleware
	beego.RunWithMiddleWares("localhost:8082", globalTracer.HttpMiddleWare)
}

type pongController struct{ baseController }

func (c *pongController) Pong() {

	time.Sleep(time.Second * 1)
	var ctx = c.Ctx.Request.Context()
	var err error
	var strBody = "pong"
	if ctx, err = otherService(ctx, "service-b", "example"); err != nil {
		strBody = err.Error()
	}

	c.Success(ctx, []byte(strBody))
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
