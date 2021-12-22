package qoi_test

import (
	"bytes"
	"image/color"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/takeyourhatoff/qoi"
)

func TestDecode(t *testing.T) {
	tt := require.New(t)
	files, err := filepath.Glob("testdata/qoi_test_images/*.qoi")
	tt.NoError(err)
	for _, p := range files {
		f, err := os.Open(p)
		tt.NoError(err)
		defer f.Close()
		img, err := qoi.Decode(f)
		tt.NoError(err, p)
		w, err := os.Open(strings.TrimSuffix(p, filepath.Ext(p)) + ".png")
		tt.NoError(err)
		defer w.Close()
		ref, err := png.Decode(w)
		tt.NoError(err)
		tt.Equal(ref.Bounds(), img.Bounds())
		for x := ref.Bounds().Min.X; x < ref.Bounds().Dx(); x++ {
			for y := ref.Bounds().Min.X; y < ref.Bounds().Dy(); y++ {
				tt.Equal(
					color.NRGBAModel.Convert(ref.At(x, y)),
					img.At(x, y),
					"%q {x: %d, y: %d}", p, x, y,
				)
			}
		}
	}
}

func TestEncode(t *testing.T) {
	tt := require.New(t)
	files, err := filepath.Glob("testdata/qoi_test_images/*.png")
	tt.NoError(err)
	for _, p := range files {
		f, err := os.Open(p)
		tt.NoError(err)
		defer f.Close()
		img, err := png.Decode(f)
		tt.NoError(err, p)
		var buf bytes.Buffer
		err = qoi.Encode(&buf, img)
		tt.NoError(err)
		ref, err := os.ReadFile(strings.TrimSuffix(p, filepath.Ext(p)) + ".qoi")
		buf.Bytes()[12], ref[12] = 0, 0 // ignore channels field in header
		tt.NoError(err)
		tt.Equal(ref, buf.Bytes())
	}
}

func BenchmarkDecode(b *testing.B) {
	tt := require.New(b)
	buf, err := os.ReadFile("testdata/qoi_test_images/dice.qoi")
	tt.NoError(err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := qoi.Decode(bytes.NewReader(buf))
		tt.NoError(err)
	}
}

func BenchmarkEncode(b *testing.B) {
	tt := require.New(b)
	f, err := os.Open("testdata/qoi_test_images/dice.png")
	tt.NoError(err)
	defer f.Close()
	img, err := png.Decode(f)
	tt.NoError(err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = qoi.Encode(ioutil.Discard, img)
		tt.NoError(err)
	}
}
