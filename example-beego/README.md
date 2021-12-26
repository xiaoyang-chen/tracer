# tracer example-beego usage

1. docker pull jaegertracing/all-in-one:latest 
2. docker run -d -p 16686:16686 -p 14268:14268 jaegertracing/all-in-one:latest
3. cd ./service-a && go run service-a.go
4. cd ./service-b && go run service-b.go
5. curl -v http://127.0.0.1:8081/ping
6. 用浏览器访问 http://127.0.0.1:16686/trace 查看上报的trace信息(确认service-a的globalTracerSwitch和service-b的globalTracerSwitch是否开启, 未开启的话请开启)
