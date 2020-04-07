package utils

import "os"

type H264FileOps struct {
	buf       []byte
	timeStamp uint32
	offset    int
	file      *os.File
}

func Newh264FileOps(file string) *H264FileOps {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	return &H264FileOps{buf: make([]byte, 1024*1024), timeStamp: 0, offset: 0, file: f}
}

func (h *H264FileOps) Close() {
	h.file.Close()
}

func (h *H264FileOps) GetFrame() []byte {
	data := h.buf[h.offset:]
	n, err := h.file.Read(data)
	if err != nil {
		h.file.Close()
		return nil
	}
	h.offset += n

	frameEndOffset := 0
	for frameEndOffset < h.offset-4 {
		if !(h.buf[0] == 0 && h.buf[1] == 0 && h.buf[2] == 0 && h.buf[3] == 1) {
			return nil
		}
		frameEndOffset++
		if h.buf[frameEndOffset] == 0 && h.buf[frameEndOffset+1] == 0 && h.buf[frameEndOffset+2] == 0 && h.buf[frameEndOffset+3] == 1 {
			break
		}
	}
	frame := make([]byte, frameEndOffset-4)
	copy(frame, h.buf[4:frameEndOffset])
	copy(h.buf[:h.offset-frameEndOffset], h.buf[frameEndOffset:h.offset])
	h.offset = h.offset - frameEndOffset
	return frame
}
