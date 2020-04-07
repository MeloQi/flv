package rtmp

import (
	"gitee.com/bdqi/gostream/flv/utils"
	"testing"
)

func TestRtmpServer(t *testing.T) {
	m := &utils.ModelsDemo{`E:\workspace\rtmp_flv_hls\flv\test.h264`}
	s := NewRtmpServer(":1935", m)
	if s == nil {
		t.Failed()
		return
	}
	err := s.Start()
	if err != nil {
		t.Log(err)
		t.Failed()
		return
	}
}
