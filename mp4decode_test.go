package mp4decode

import (
	"reflect"
	"testing"
)

func Test_avctoAnnexB(t *testing.T) {
	type args struct {
		buf        []byte
		lengthsize int
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{"", args{[]byte{0, 0, 0, 2, 0, 1, 0, 0, 0, 3, 1, 1, 1}, 4}, []byte{0, 0, 0, 1, 0, 1, 0, 0, 0, 1, 1, 1, 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := avctoAnnexB(tt.args.buf, tt.args.lengthsize)
			if (err != nil) != tt.wantErr {
				t.Errorf("avctoAnnexB() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("avctoAnnexB() = %v, want %v", got, tt.want)
			}
		})
	}
}
