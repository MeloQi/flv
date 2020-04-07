package muxer

import (
	"github.com/MeloQi/flv/utils"
	stream "github.com/MeloQi/interfaces"
	"os"
	"testing"
)

func TestFLVMuxer_Muxer(t *testing.T) {
	timeStamp := uint32(0)
	in := utils.Newh264FileOps(`E:\workspace\rtmp_flv_hls\flv\test.h264`)
	if in == nil {
		t.Log("err ")
		t.Fail()
		return
	}
	out, err := os.Create(`E:\workspace\rtmp_flv_hls\flv\test.flv`)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	defer func() {
		out.Close()
		in.Close()
	}()
	flv := NewFLVMuxer(out, true, false)
	for {
		frame := in.GetFrame()
		if frame == nil {
			break
		}

		p := stream.Packet{IsMetadata: false, IsVideo: true, TimeStamp: timeStamp, Data: []stream.FrameSlice{{frame, nil}}}
		if err := flv.Muxer(&p); err != nil {
			t.Log(err)
			t.Fail()
			return
		}
		if !(frame[0]&0x1f == 7 || frame[0]&0x1f == 8) {
			timeStamp += 40
		}
	}
}
