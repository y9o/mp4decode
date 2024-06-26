# mp4decode

mp4 decode at [go-openh264](https://github.com/y9o/go-openh264/) 

# Windows example

`ffmpeg -f lavfi -i testsrc -vf "scale=out_color_matrix=bt601:out_range=pc,format=yuv420p" -t 5 testdata/testsrc.mp4`

```
import (
	"bytes"
	"fmt"
	"image/jpeg"
	"log"
	"os"
	"time"

	"github.com/y9o/go-openh264"
	"github.com/y9o/mp4decode"
)

func main(){
	if err := openh264.Open("openh264-2.4.1-win64.dll"); err != nil {
		log.Fatal(err)
	}
	defer openh264.Close()
	dec, err := mp4decode.NewFromFile("testdata/testsrc.mp4")
	if err != nil {
		log.Fatal(err)
	}
	defer dec.Close()

	if err := dec.Initialize(); err != nil {
		log.Fatal(err)
	}

	if err := dec.SeekByTime(4 * time.Second); err != nil {
		log.Fatal(err)
	}

	jpgdata := bytes.NewBuffer(make([]byte, 0))
	for {
		yuv, err := dec.Read()
		if err != nil {
			if err != mp4decode.ErrEndOfStream {
				log.Fatal(err)
			}
			break
		}
		jpeg.Encode(jpgdata, yuv, &jpeg.Options{Quality: 95})
		os.WriteFile("4.jpg", jpgdata.Bytes(), 0600)
		break
	}
}
```