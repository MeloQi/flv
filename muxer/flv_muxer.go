package muxer

import (
	"github.com/MeloQi/flv/amf"
	"github.com/MeloQi/flv/av"
	"github.com/MeloQi/flv/utils/pio"
	stream "github.com/MeloQi/interfaces"
	"fmt"
	"io"
)

const (
	headerLen = 11
)

type FLVMuxer struct {
	ctx       io.Writer
	flvHeader []byte
	videoTag  *VideoTag
	buf       []byte
}

func NewFLVMuxer(ctx io.Writer, hasVideo, hasAudio bool) *FLVMuxer {
	if !hasAudio && !hasVideo {
		return nil
	}
	flag := byte(0)
	if hasAudio {
		flag |= 0x04
	}
	if hasVideo {
		flag |= 0x01
	}
	ret := &FLVMuxer{
		ctx:       ctx,
		videoTag:  NewVideoTag(),
		flvHeader: []byte{0x46, 0x4c, 0x56, 0x01, flag, 0x00, 0x00, 0x00, 0x09},
		buf:       make([]byte, headerLen),
	}

	ret.ctx.Write(ret.flvHeader)
	pio.PutI32BE(ret.buf[:4], 0)
	ret.ctx.Write(ret.buf[:4])

	return ret
}

func (flvMuxer *FLVMuxer) Muxer(p *stream.Packet) error {
	if p == nil || p.Data == nil || len(p.Data) == 0 {
		return fmt.Errorf("Packet is empty")
	}
	h := flvMuxer.buf[:headerLen]
	typeID := av.TAG_VIDEO
	if !p.IsVideo {
		if p.IsMetadata {
			var err error
			typeID = av.TAG_SCRIPTDATAAMF0
			p.Data[0].Data, err = amf.MetaDataReform(p.Data[0].Data, amf.DEL)
			if err != nil {
				return err
			}
		} else {
			typeID = av.TAG_AUDIO
		}
	}

	if p.IsVideo && !flvMuxer.videoTag.AddHeaderToVideoTagData(p) {
		return nil
	}

	dataLen := 0
	for _, d := range p.Data {
		dataLen += len(d.Data)
	}
	timestamp := p.TimeStamp
	preDataLen := dataLen + headerLen
	timestampbase := timestamp & 0xffffff
	timestampExt := timestamp >> 24 & 0xff

	pio.PutU8(h[0:1], uint8(typeID))
	pio.PutI24BE(h[1:4], int32(dataLen))
	pio.PutI24BE(h[4:7], int32(timestampbase))
	pio.PutU8(h[7:8], uint8(timestampExt))

	if _, err := flvMuxer.ctx.Write(h); err != nil {
		return err
	}
	for _, d := range p.Data {
		if _, err := flvMuxer.ctx.Write(d.Data); err != nil {
			return err
		}
	}

	pio.PutI32BE(h[:4], int32(preDataLen))
	if _, err := flvMuxer.ctx.Write(h[:4]); err != nil {
		return err
	}

	return nil
}

type VideoTag struct {
	//video
	spsPPSHeader  []byte
	videoHeader   []byte
	waitPPS       bool
	spsPPSIsSend  bool
	spsPPSTagData []stream.FrameSlice
	spsLen        []byte
	ppsCnt        []byte
	ppsLen        []byte
	videoLen      []byte
}

func NewVideoTag() *VideoTag {
	return &VideoTag{
		spsPPSHeader: []byte{0x17, 0x00, 0x00, 0x00, 0x00, 0x01, 0xff, 0x00, 0xff, 0xFF, 0xE1},
		videoHeader:  []byte{0x17, 0x01, 0x00, 0x00, 0x00},
		spsLen:       []byte{0x00, 0x00},
		ppsCnt:       []byte{0x01},
		ppsLen:       []byte{0x00, 0x00},
		videoLen:     []byte{0x00, 0x00, 0x00, 0x00},
		waitPPS:      false,
		spsPPSIsSend: false,
	}
}

func (tag *VideoTag) AddHeaderToVideoTagData(p *stream.Packet) (isContinue bool) {
	isContinue = true

	frameLen := 0
	for _, d := range p.Data {
		frameLen += len(d.Data)
	}
	frame0 := p.Data[0].Data
	if frame0[0]&0x1f == 7 { //sps
		pkg := []stream.FrameSlice{}
		tag.spsPPSHeader[6] = frame0[1]
		tag.spsPPSHeader[7] = frame0[2]
		tag.spsPPSHeader[8] = frame0[3]
		pkg = append(pkg, stream.FrameSlice{tag.spsPPSHeader,nil})
		pio.PutI16BE(tag.spsLen, int16(frameLen))
		pkg = append(pkg, stream.FrameSlice{tag.spsLen,nil})
		pkg = append(pkg, p.Data...)
		tag.waitPPS = true
		tag.spsPPSTagData = pkg
		return false
	} else if frame0[0]&0x1f == 8 { //pps
		if !tag.waitPPS {
			tag.spsPPSTagData = []stream.FrameSlice{}
			return false
		}
		tag.waitPPS = false
		pkg := tag.spsPPSTagData
		pkg = append(pkg, stream.FrameSlice{tag.ppsCnt,nil})
		pio.PutI16BE(tag.ppsLen, int16(frameLen))
		pkg = append(pkg, stream.FrameSlice{tag.ppsLen,nil})
		pkg = append(pkg, p.Data...)
		tag.spsPPSIsSend = true
		p.Data = pkg
	} else if frame0[0]&0x1f == 6 {
		return false
	} else {
		if !tag.spsPPSIsSend {
			return false
		}
		tag.videoHeader[0] = 0x27
		if frame0[0]&0x1f == 5 {
			tag.videoHeader[0] = 0x17
		}
		//flvMuxer.videoHeader[4] = 00 //40ms 帧间隔 dts与pts的差，有B帧时非零，其他情况为0
		//flvMuxer.videoHeader[4] = 40 //40ms 帧间隔
		pkg := []stream.FrameSlice{}
		pkg = append(pkg, stream.FrameSlice{tag.videoHeader,nil})
		pio.PutI32BE(tag.videoLen, int32(frameLen))
		pkg = append(pkg, stream.FrameSlice{tag.videoLen,nil})
		pkg = append(pkg, p.Data...)
		p.Data = pkg
	}

	return true
}
