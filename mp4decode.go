// Package mp4decode provides for MP4 Video decoding
package mp4decode

import (
	"errors"
	"fmt"
	"image"
	"io"
	"time"
	"unsafe"

	"github.com/y9o/mp4read"

	"github.com/y9o/go-openh264"
)

type Mp4dec struct {
	mp4read                 *mp4read.Mp4read
	sampleBuffer            []byte
	currentSample           *mp4read.VideoSampleInfo
	samplesinDecoder        map[int64]mp4read.VideoSampleInfo
	Decoder                 *openh264.ISVCDecoder
	DecParam                *openh264.SDecodingParam
	DstBufInfo              *openh264.SBufferInfo
	frameNumber             int64
	seekframe               *[3][]byte
	num_of_frames_in_buffer int
	openh264Initialize      bool
}

var ErrEndOfStream = fmt.Errorf("end of stream")

// NewFromFile
//
// mp4ファイルを読み込み。
func NewFromFile(mp4filename string) (*Mp4dec, error) {
	fh, err := mp4read.NewFromFile(mp4filename)
	if err != nil {
		return nil, err
	}

	ret := &Mp4dec{
		mp4read: fh,
	}
	if err := ret.init(); err != nil {
		return nil, err
	}
	return ret, nil
}

// NewFromReadSeeker
//
// mp4を読み込み。
func NewFromReadSeeker(r io.ReadSeeker) (*Mp4dec, error) {
	fh, err := mp4read.NewFromReadSeeker(r)
	if err != nil {
		return nil, err
	}
	ret := &Mp4dec{
		mp4read: fh,
	}
	if err := ret.init(); err != nil {
		return nil, err
	}

	return ret, nil
}

// init
//
// openh264のデコーダーを作成
func (v *Mp4dec) init() error {
	var ppdec *openh264.ISVCDecoder
	if ret := openh264.WelsCreateDecoder(&ppdec); ret != 0 || ppdec == nil {
		return fmt.Errorf("WelsCreateDecoder: %d", ret)
	}
	v.Decoder = ppdec
	v.DecParam = &openh264.SDecodingParam{}
	v.DecParam.UiTargetDqLayer = 0xff
	v.DecParam.EEcActiveIdc = openh264.ERROR_CON_SLICE_MV_COPY_CROSS_IDR_FREEZE_RES_CHANGE
	v.DecParam.SVideoProperty.Size = uint32(unsafe.Sizeof(v.DecParam.SVideoProperty))
	v.DecParam.SVideoProperty.EVideoBsType = openh264.VIDEO_BITSTREAM_DEFAULT
	v.DstBufInfo = &openh264.SBufferInfo{}

	v.samplesinDecoder = make(map[int64]mp4read.VideoSampleInfo)
	v.frameNumber = -1

	return nil
}

// Close
//
// 必要な後処理。readで返されたメモリにアクセスできなくなる。
func (v *Mp4dec) Close() error {
	if v.Decoder != nil {
		if v.openh264Initialize {
			v.Decoder.Uninitialize()
			v.openh264Initialize = false
		}
		openh264.WelsDestroyDecoder(v.Decoder)
		v.Decoder = nil
	}
	if v.mp4read != nil {
		err := v.mp4read.Close()
		v.mp4read = nil
		if err != nil {
			return err
		}
	}

	return nil
}

// Duration
//
// Videoの長さ。1sec==Timescale()
func (v *Mp4dec) Duration() int64 {
	if v.mp4read == nil {
		return 0
	}
	return v.mp4read.Duration()
}

// TimeDuration
//
// Videoの長さ
func (v *Mp4dec) TimeDuration() time.Duration {
	if v.mp4read == nil {
		return 0
	}
	return v.mp4read.TimeDuration()
}

// Timescale
//
// Videoの1秒の単位。
func (v *Mp4dec) Timescale() uint32 {
	if v.mp4read == nil {
		return 0
	}
	return v.mp4read.Timescale()
}

// VideoSize
//
// ビデオの情報
func (v *Mp4dec) VideoSize() (width, height, samples int, err error) {
	if v.mp4read == nil {
		return -1, -1, -1, errors.New("video not found")
	}
	if info, err := v.mp4read.VideoInfo(); err != nil {
		return -1, -1, -1, err
	} else {
		return info.Width, info.Height, info.Samples, nil
	}
}

// Initialize
//
// mp4ヘッダを解析したり、openh264を初期化したりします。
func (v *Mp4dec) Initialize() error {
	if err := v.mp4read.Initialize(); err != nil {
		return err
	}
	return v.decoderInit()
}

// decoderInit
//
// decoderの初期化。初期化済みなら解除してから。SPSPPS情報もデコーダーに渡す。
func (v *Mp4dec) decoderInit() error {
	if v.openh264Initialize {
		v.Decoder.Uninitialize()
		v.openh264Initialize = false
	}
	if r := v.Decoder.Initialize(v.DecParam); r != 0 {
		return fmt.Errorf("Initialize: %d", r)
	}
	v.openh264Initialize = true

	spspps := v.mp4read.GetSPSPPS()
	if len(spspps) > 0 {
		l := 0
		for _, v := range spspps {
			l += 4 + len(v)
		}
		annexb := make([]byte, 0, l)
		for _, v := range spspps {
			annexb = append(annexb, []byte{0, 0, 0, 1}...)
			annexb = append(annexb, v...)
		}

		var pDst [3][]byte
		if r := v.Decoder.DecodeFrame2(annexb, len(annexb), &pDst, v.DstBufInfo); r != 0 {
			return fmt.Errorf("DecodeFrame2: %d", r)
		}
	}
	return nil
}

// SeekByTime
//
// 指定時刻に移動。
func (v *Mp4dec) SeekByTime(timestamp time.Duration) error {
	if v.mp4read == nil {
		return errors.New("vodeo not found")
	}

	ts := int64((float64(timestamp) / float64(time.Second)) * float64(v.mp4read.Timescale()))

	return v.Seek(ts)
}

// Seek
//
// 指定時刻に移動。1sec == Timescale()
func (v *Mp4dec) Seek(timestamp int64) error {
	if v.mp4read == nil {
		return errors.New("vodeo not found")
	}
	if !v.inDecoder(timestamp) {
		ts, err := v.mp4read.Seek(timestamp, false)
		if err != nil {
			return err
		} else if ts {
			v.flush()
			v.decoderInit() //openh264のバージョンによっては必要ない？
		}
	}
	return v.move(timestamp)
}

// inDecoder
//
// openh254デコーダー内に指定時刻が存在するか
func (v *Mp4dec) inDecoder(timestamp int64) bool {
	for _, v := range v.samplesinDecoder {
		if v.CompositionTime <= timestamp && timestamp < v.CompositionTime+int64(v.TimeDelta) {
			return true
		}
	}
	return false
}

// move
//
// 指定時刻のフレームまでデコードしてスキップ。
func (v *Mp4dec) move(timestamp int64) (e error) {
	for {
		raw, err := v.ReadRaw()
		if err != nil {
			return err
		}
		if v.currentSample == nil {
			return errors.New("cant get currentData")
		}
		if v.currentSample.CompositionTime+int64(v.currentSample.TimeDelta) > timestamp {
			v.seekframe = raw
			break
		}
	}
	return
}

// flush
//
// openh264のデコーダーに残っているデータをフラッシュ。seekしたら必要
func (v *Mp4dec) flush() {
	v.seekframe = nil
	v.frameNumber = -1
	v.samplesinDecoder = make(map[int64]mp4read.VideoSampleInfo)
	var n int
	v.Decoder.GetOption(openh264.DECODER_OPTION_NUM_OF_FRAMES_REMAINING_IN_BUFFER, &n)
	var pDst [3][]byte
	var BufferInfo openh264.SBufferInfo
	for n > 0 {
		n--
		v.Decoder.FlushFrame(&pDst, &BufferInfo)
	}
}

// ReadRaw
//
// frameデータを返します。Videoの終わりに到達するとErrEndOfStreamを返します。openh264内のメモリを参照するので内部で解放されるとアクセスできなくなる。
func (v *Mp4dec) ReadRaw() (*[3][]byte, error) {
	if v.seekframe != nil {
		//seekで取得したframeデータがあれば返す。
		tmp := v.seekframe
		v.seekframe = nil
		return tmp, nil
	}
	if v.num_of_frames_in_buffer == -1 {
		return nil, ErrEndOfStream
	}
	var sample mp4read.VideoSampleInfo

	for {
		if err := v.mp4read.NextSample(&sample); err != nil {
			if err == mp4read.ErrEndOfStream {
				break
			}
			return nil, err
		}

		if avc, err := v.mp4read.ReadMdatAtSample(&sample, v.sampleBuffer); err != nil {
			return nil, err
		} else if annexb, err := avctoAnnexB(avc, sample.NalLengthSize); err != nil {
			return nil, err
		} else {
			v.sampleBuffer = annexb
		}
		if len(v.sampleBuffer) > 0 {
			var pDst [3][]byte
			var BufferInfo openh264.SBufferInfo
			var err error = nil
			BufferInfo.UiInBsTimeStamp = uint64(sample.Number)
			v.samplesinDecoder[sample.Number] = sample
			if r := v.Decoder.DecodeFrameNoDelay(v.sampleBuffer, len(v.sampleBuffer), &pDst, &BufferInfo); r != 0 {
				err = fmt.Errorf("DecodeFrameNoDelay: %d", r)
			}
			v.DstBufInfo = &BufferInfo
			if pDst[0] != nil && BufferInfo.IBufferStatus == 1 {
				if sampledata, ok := v.samplesinDecoder[int64(BufferInfo.UiOutYuvTimeStamp)]; ok {
					delete(v.samplesinDecoder, int64(BufferInfo.UiOutYuvTimeStamp))
					v.currentSample = &sampledata
					if v.frameNumber == -1 {
						v.frameNumber = sampledata.Number
					}
				} else {
					v.currentSample = nil
				}
				v.frameNumber++
				return &pDst, err
			} else if err != nil {
				return nil, err
			}
		}

	}

	if v.num_of_frames_in_buffer == 0 {
		var n int = 1
		v.Decoder.SetOption(openh264.DECODER_OPTION_END_OF_STREAM, &n)
		v.Decoder.GetOption(openh264.DECODER_OPTION_NUM_OF_FRAMES_REMAINING_IN_BUFFER, &n)
		v.num_of_frames_in_buffer = n
	}
	for v.num_of_frames_in_buffer > 0 {
		var BufferInfo openh264.SBufferInfo
		v.num_of_frames_in_buffer--
		if v.num_of_frames_in_buffer == 0 {
			v.num_of_frames_in_buffer = -1
		}
		var pDst [3][]byte
		var err error = nil
		BufferInfo.IBufferStatus = 0
		if r := v.Decoder.FlushFrame(&pDst, &BufferInfo); r != 0 {
			err = fmt.Errorf("FlushFrame: %d", r)
		}
		v.DstBufInfo = &BufferInfo
		if pDst[0] != nil && BufferInfo.IBufferStatus == 1 {
			if data, ok := v.samplesinDecoder[int64(BufferInfo.UiOutYuvTimeStamp)]; ok {
				delete(v.samplesinDecoder, int64(BufferInfo.UiOutYuvTimeStamp))
				v.currentSample = &data
			} else {
				v.currentSample = nil
			}
			v.frameNumber++
			return &pDst, err
		} else if err != nil {
			return nil, err
		}
	}
	v.num_of_frames_in_buffer = -1
	return nil, ErrEndOfStream
}

// Read
//
// ReadRawのデータをimage.YCbCrでラッピング。 colormatrixは無視
// openh264内のメモリを参照するので内部で解放されるとアクセスできなくなる。
func (v *Mp4dec) Read() (*image.YCbCr, error) {
	buf, err := v.ReadRaw()
	if buf == nil {
		return nil, err
	}
	t := v.DstBufInfo.UsrData_sSystemBuffer()
	if t.IFormat != openh264.VideoFormatI420 {
		return nil, fmt.Errorf("not support format: %d", t.IFormat)
	}
	yuv := &image.YCbCr{
		Y:              buf[0],
		Cb:             buf[1],
		Cr:             buf[2],
		YStride:        int(t.IStride[0]),
		CStride:        int(t.IStride[1]),
		Rect:           image.Rect(0, 0, int(t.IWidth), int(t.IHeight)),
		SubsampleRatio: image.YCbCrSubsampleRatio420,
	}
	return yuv, err

}

// CurrentFrameNumber first frame: 1
//
// seekした場合は推測値
func (v *Mp4dec) CurrentFrameNumber() int64 {
	return v.frameNumber
}

// CurrentCompositionTime
//
//	first frame: 0
//	error: -1
func (v *Mp4dec) CurrentCompositionTime() int64 {
	if v.currentSample == nil {
		return -1
	}
	return v.currentSample.CompositionTime
}

// CurrentTimeDelta
//
//	error: -1
func (v *Mp4dec) CurrentTimeDelta() int64 {
	if v.currentSample == nil {
		return -1
	}
	return int64(v.currentSample.TimeDelta)
}

// avctoAnnexB
//
// Byte stream format の変換。
func avctoAnnexB(buf []byte, lengthsize int) ([]byte, error) {
	//スタートコードが4バイト以外だとこの関数では使いづらい。
	if lengthsize != 4 {
		return buf, fmt.Errorf("not support LengthSize: %d", lengthsize)
	}

	var length uint32
	for off := 0; off+lengthsize < len(buf); {
		for i := 0; i < lengthsize; i++ {
			length = (length << 8) + uint32(buf[off+i])
			buf[off+i] = 0
		}
		buf[off+lengthsize-1] = 1
		off += lengthsize + int(length)
	}
	return buf, nil
}
