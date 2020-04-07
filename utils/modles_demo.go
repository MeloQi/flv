package utils

import (
	stream "github.com/MeloQi/interfaces"
	"log"
	"time"
)

type ModelsDemo struct {
	H264File string
}

func (m *ModelsDemo) GetStream(needMediaInfo stream.MediaInfo) *stream.Stream {
	src := Newh264FileOps(m.H264File)
	if src == nil {
		return nil
	}
	media := stream.NewStream(true, false, stream.MediaInfo{})
	go func() {
		defer func() {
			src.Close()
			log.Print("stream exit")
			if r := recover(); r != nil {
			}
		}()
		timeStamp := uint32(0)
		for {
			frame := src.GetFrame()
			if frame == nil || len(frame) == 0 {
				log.Print("stream end")
				media.Close()
				return
			}
			if !media.Send(&stream.Packet{IsMetadata: false, IsVideo: true, TimeStamp: timeStamp, Data: []stream.FrameSlice{{frame, nil}}}) {
				return
			}
			if !(frame[0]&0x1f == 7 || frame[0]&0x1f == 8) {
				timeStamp += 40
				time.Sleep(time.Millisecond * 39)
			}
		}
	}()
	return media
}
