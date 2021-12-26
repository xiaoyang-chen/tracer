package tracer

import (
	"context"
	"reflect"
	"testing"
)

func TestMakeupUrlByHostPathQueryParams(t *testing.T) {
	type args struct {
		host        string
		path        string
		queryParams map[string]string
	}
	tests := []struct {
		name    string
		args    args
		wantUrl string
	}{
		{
			name: "baidu 1",
			args: args{
				host: "https://www.baidu.com",
				path: "/s",
				queryParams: map[string]string{
					"ie":        "utf-8",
					"f":         "8",
					"rsv_bp":    "1",
					"rsv_idx":   "1",
					"tn":        "baidu",
					"wd":        "asd",
					"fenlei":    "256",
					"rsv_pq":    "a8b7a51c0004bc91",
					"rsv_t":     "2f9bKHuIfAKAID4E6%2FeFoWh4powNiruueskvSLJsVyiDiGLpSOC4Yr8hNJw",
					"rqlang":    "cn",
					"rsv_enter": "1",
					"rsv_dl":    "tb",
					"rsv_sug3":  "4",
					"rsv_sug1":  "2",
					"rsv_sug7":  "101",
					"rsv_sug2":  "0",
					"rsv_btype": "i",
					"prefixsug": "asd",
					"rsp":       "9",
					"inputT":    "31418",
					"rsv_sug4":  "31418",
				},
			},
			wantUrl: "https://www.baidu.com/s?ie=utf-8&f=8&rsv_bp=1&rsv_idx=1&tn=baidu&wd=asd&fenlei=256&rsv_pq=a8b7a51c0004bc91&rsv_t=2f9bKHuIfAKAID4E6%2FeFoWh4powNiruueskvSLJsVyiDiGLpSOC4Yr8hNJw&rqlang=cn&rsv_enter=1&rsv_dl=tb&rsv_sug3=4&rsv_sug1=2&rsv_sug7=101&rsv_sug2=0&rsv_btype=i&prefixsug=asd&rsp=9&inputT=31418&rsv_sug4=31418",
		},
		{
			name: "http://127.0.0.1:8080/path/xxx, nil queryParams",
			args: args{
				host:        "http://127.0.0.1:8080",
				path:        "/path/xxx",
				queryParams: nil,
			},
			wantUrl: "http://127.0.0.1:8080/path/xxx",
		},
		{
			name: "http://127.0.0.1:8080/path/xxx, empty queryParams",
			args: args{
				host:        "http://127.0.0.1:8080",
				path:        "/path/xxx",
				queryParams: make(map[string]string),
			},
			wantUrl: "http://127.0.0.1:8080/path/xxx",
		},
		{
			name: `http://localhost:18200/path/xxx, queryParams: map[string]string{ "abc": "213123", "def": "213123" }`,
			args: args{
				host:        "http://localhost:18200",
				path:        "/path/xxx",
				queryParams: map[string]string{"abc": "213123", "def": "213123"},
			},
			wantUrl: "http://localhost:18200/path/xxx?abc=213123&def=213123",
		},
	}
	var ti = InitEmptyTracer()
	var gotBody, wantBody []byte
	var err error
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotUrl := MakeupUrlByHostPathQueryParams(tt.args.host, tt.args.path, tt.args.queryParams); gotUrl != tt.wantUrl {
				if len(gotUrl) != len(tt.wantUrl) {
					t.Errorf("MakeupUrlByHostPathQueryParams() = %v, want %v", gotUrl, tt.wantUrl)
					return
				}
				if _, gotBody, err = ti.GetFasthttp(
					context.Background(), gotUrl, nil, nil,
				); err != nil {
					t.Errorf("MakeupUrlByHostPathQueryParams() = %v, want %v", gotUrl, tt.wantUrl)
					return
				}
				if _, wantBody, err = ti.GetFasthttp(
					context.Background(), tt.wantUrl, nil, nil,
				); err != nil {
					t.Errorf("MakeupUrlByHostPathQueryParams() = %v, want %v", gotUrl, tt.wantUrl)
					return
				}
				if !reflect.DeepEqual(gotBody, wantBody) {
					t.Errorf("MakeupUrlByHostPathQueryParams() = %v, want %v", gotUrl, tt.wantUrl)
				}
			}
		})
	}
}
