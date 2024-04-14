package mp4decode_test

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"log"
	"time"

	"github.com/y9o/go-openh264"
	"github.com/y9o/mp4decode"
)

func ExampleNewFromFile() {
	if err := openh264.Open("openh264-2.4.1-win64.dll"); err != nil {
		log.Fatal(err)
	}
	defer openh264.Close()
	dec, err := mp4decode.NewFromFile("testdata/testsrc.mp4")
	if err != nil {
		log.Fatal(err)
	}

	if err := dec.Initialize(); err != nil {
		log.Fatal(err)
	}

	if err := dec.SeekByTime(4 * time.Second); err != nil {
		log.Fatal(err)
	}

	jpgdata := bytes.NewBuffer(make([]byte, 0, 12757))
	for {
		yuv, err := dec.Read()
		if err != nil {
			if err != mp4decode.ErrEndOfStream {
				log.Fatal(err)
			}
			break
		}

		jpeg.Encode(jpgdata, yuv, &jpeg.Options{Quality: 95})
		break
	}
	fmt.Println("len:", jpgdata.Len())
	//	os.WriteFile("4.jpg", jpgdata.Bytes(), 0600)

	if err := dec.Close(); err != nil {
		log.Fatal(err)
	}
	// Output: len: 12757
}
