package rtmp

import (
	"github.com/MeloQi/flv/amf"
	"github.com/MeloQi/flv/av"
	"github.com/MeloQi/flv/muxer"
	"github.com/MeloQi/flv/rtmp/core"
	"github.com/MeloQi/flv/utils"
	"bytes"
	"errors"
	"flag"
	"fmt"
	stream "github.com/MeloQi/interfaces"
	go_h264 "github.com/MeloQi/go-h264"
	"log"
	"net"
	"net/url"
	"reflect"
	"strings"
	"time"
)

const (
	maxQueueNum           = 1024
	SAVE_STATICS_INTERVAL = 5000
)

var (
	readTimeout  = flag.Int("readTimeout", 10, "read time out")
	writeTimeout = flag.Int("writeTimeout", 10, "write time out")
)

type Server struct {
	addr   string
	m      stream.Modles
	Stoped bool
	listen net.Listener
}

func NewRtmpServer(addr string, m stream.Modles) *Server {
	return &Server{addr: addr, m: m, Stoped: false, listen: nil}
}

func (s *Server) Stop() {
	defer func() {
		recover()
	}()
	if s.Stoped {
		return
	}
	s.Stoped = true
	if s.listen != nil {
		s.listen.Close()
	}
}

func (s *Server) Start() error {
	listen, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listen = listen
	log.Println("rtmp server start on ", listen.Addr().String())
	return s.Serve(listen)
}

func (s *Server) Serve(listener net.Listener) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("rtmp serve panic: ", r)
		}
	}()

	for {
		var netconn net.Conn
		netconn, err = listener.Accept()
		if err != nil {
			return
		}
		conn := core.NewConn(netconn, 4*1024)
		log.Println("new client, connect remote:", conn.RemoteAddr().String(),
			"local:", conn.LocalAddr().String())
		//netconn.(*net.TCPConn).SetWriteBuffer(20 * 1024 * 1024)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn *core.Conn) error {

	if err := conn.HandshakeServer(); err != nil {
		conn.Close()
		log.Println("handleConn HandshakeServer err:", err)
		return err
	}
	connServer := core.NewConnServer(conn)

	if err := connServer.ReadMsg(); err != nil {
		conn.Close()
		log.Println("handleConn read msg err:", err)
		return err
	}

	log.Printf("handleConn: IsPublisher=%v", connServer.IsPublisher())
	if connServer.IsPublisher() {
		log.Println("Is Publisher, not support")
		conn.Close()
		return nil
	} else {
		_, _, url := connServer.GetInfo()
		id, err := stream.StreamID(url)
		if err != nil {
			err := fmt.Errorf("invalid url path")
			log.Println(err)
			return err
		}
		media := s.m.GetStream(stream.MediaInfo{IsLive: true, StreamID: id, Vcodec: stream.H264Codec, Acodec: stream.AACCodec})
		if media == nil {
			err := fmt.Errorf("Not Found")
			log.Println(err)
			return err
		}
		writer := NewVirWriter(connServer, media)
		log.Printf("new player: %+v\n", writer.Info())
	}

	return nil
}

type GetInFo interface {
	GetInfo() (string, string, string)
}

type StreamReadWriteCloser interface {
	GetInFo
	Close(error)
	Write(core.ChunkStream) error
	Read(c *core.ChunkStream) error
}

type StaticsBW struct {
	StreamId               uint32
	VideoDatainBytes       uint64
	LastVideoDatainBytes   uint64
	VideoSpeedInBytesperMS uint64

	AudioDatainBytes       uint64
	LastAudioDatainBytes   uint64
	AudioSpeedInBytesperMS uint64

	LastTimestamp int64
}

type VirWriter struct {
	Uid         string
	closed      bool
	conn        StreamReadWriteCloser
	WriteBWInfo StaticsBW
	media       *stream.Stream
	videoTag    *muxer.VideoTag
}

func NewVirWriter(conn StreamReadWriteCloser, media *stream.Stream) *VirWriter {
	ret := &VirWriter{
		Uid:         utils.NewId(),
		conn:        conn,
		media:       media,
		videoTag:    muxer.NewVideoTag(),
		WriteBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
	}

	go ret.Check()
	go func() {
		err := ret.SendPacket()
		if err != nil {
			log.Println(err)
		}
	}()
	return ret
}

func (v *VirWriter) SaveStatics(streamid uint32, length uint64, isVideoFlag bool) {
	nowInMS := int64(time.Now().UnixNano() / 1e6)

	v.WriteBWInfo.StreamId = streamid
	if isVideoFlag {
		v.WriteBWInfo.VideoDatainBytes = v.WriteBWInfo.VideoDatainBytes + length
	} else {
		v.WriteBWInfo.AudioDatainBytes = v.WriteBWInfo.AudioDatainBytes + length
	}

	if v.WriteBWInfo.LastTimestamp == 0 {
		v.WriteBWInfo.LastTimestamp = nowInMS
	} else if (nowInMS - v.WriteBWInfo.LastTimestamp) >= SAVE_STATICS_INTERVAL {
		diffTimestamp := (nowInMS - v.WriteBWInfo.LastTimestamp) / 1000

		v.WriteBWInfo.VideoSpeedInBytesperMS = (v.WriteBWInfo.VideoDatainBytes - v.WriteBWInfo.LastVideoDatainBytes) * 8 / uint64(diffTimestamp) / 1000
		v.WriteBWInfo.AudioSpeedInBytesperMS = (v.WriteBWInfo.AudioDatainBytes - v.WriteBWInfo.LastAudioDatainBytes) * 8 / uint64(diffTimestamp) / 1000

		v.WriteBWInfo.LastVideoDatainBytes = v.WriteBWInfo.VideoDatainBytes
		v.WriteBWInfo.LastAudioDatainBytes = v.WriteBWInfo.AudioDatainBytes
		v.WriteBWInfo.LastTimestamp = nowInMS
	}
}

func (v *VirWriter) Check() {
	var c core.ChunkStream
	for {
		if err := v.conn.Read(&c); err != nil {
			v.Close(err)
			return
		}
	}
}

func (v *VirWriter) SendPacket() error {
	Flush := reflect.ValueOf(v.conn).MethodByName("Flush")
	var cs core.ChunkStream
	isFirst := true
	for {
		p, ok := <-v.media.FrameChan
		if ok {
			for isFirst && p.Data[0].Data[0]&0x1f == 7 { //first sps, add amf
				w, h, err := go_h264.GetWidthHeight(p.Data[0].Data)
				if err != nil {
					break
				}
				buf := new(bytes.Buffer)
				enc := new(amf.Encoder)
				if _, err = enc.EncodeAmf0(buf, "onMetaData"); err != nil {
					break
				}
				obj := make(amf.Object)
				obj["MetaDataCreator"] = "bdqi"
				obj["hasVideo"] = true
				obj["hasAudio"] = false
				obj["hasMatadata"] = true
				obj["canSeekToEnd"] = false
				obj["duration"] = 0.0
				obj["width"] = float64(w)
				obj["height"] = float64(h)
				obj["videocodecid"] = 7.0
				obj["filesize"] = 0.0
				if _, err = enc.EncodeAmf0EcmaArray(buf, obj, true); err != nil {
					break
				}
				cs.TypeID = av.TAG_SCRIPTDATAAMF0
				cs.Data = buf.Bytes()
				cs.DataSlices = nil
				cs.Timestamp = 0
				if err = v.conn.Write(cs); err != nil {
					v.Close(err)
					return err
				}
				Flush.Call(nil)

				isFirst = false
			}

			if p.IsVideo {
				cs.TypeID = av.TAG_VIDEO
				cs.StreamID = 1
			} else {
				if p.IsMetadata {
					cs.TypeID = av.TAG_SCRIPTDATAAMF0
					cs.StreamID = 2
				} else {
					cs.TypeID = av.TAG_AUDIO
					cs.StreamID = 3
				}
			}

			if p.IsVideo && !v.videoTag.AddHeaderToVideoTagData(p) {
				continue
			}

			cs.Data = nil
			cs.DataSlices = p.Data
			cs.Timestamp = p.TimeStamp
			err := v.conn.Write(cs)
			if err != nil {
				v.Close(err)
				return err
			}
			Flush.Call(nil)
		} else {
			err := errors.New("closed")
			v.Close(err)
			return err
		}

	}
	return nil
}

func (v *VirWriter) Info() (ret av.Info) {
	ret.UID = v.Uid
	_, _, URL := v.conn.GetInfo()
	ret.URL = URL
	_url, err := url.Parse(URL)
	if err != nil {
		log.Println(err)
	}
	ret.Key = strings.TrimLeft(_url.Path, "/")
	ret.Inter = true
	return
}

func (v *VirWriter) Close(err error) {
	defer func() {
		if r := recover(); r != nil {
		}
	}()
	log.Println("player ", v.Info(), "closed: "+err.Error())
	v.closed = true
	v.conn.Close(err)
	v.media.Close()
}
