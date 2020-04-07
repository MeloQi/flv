package httpflv

import (
	"gitee.com/bdqi/gostream/flv/utils"
	"testing"
)

func TestHttpFlvServer(t *testing.T) {
	m := &utils.ModelsDemo{`E:\workspace\rtmp_flv_hls\flv\test.h264`}
	s := NewHttpFlvServer(":8888", m)
	if s == nil {
		t.Failed()
		return
	}
	s.Start()
}
