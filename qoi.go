package qoi

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
)

const (
	magic     = "qoif"
	endMarker = "\x00\x00\x00\x00\x00\x00\x00\x01"
)

func init() {
	image.RegisterFormat("qoi", magic, Decode, DecodeConfig)
}

func Decode(r io.Reader) (image.Image, error) {
	b := bufio.NewReader(r)
	cfg, err := DecodeConfig(b)
	if err != nil {
		return nil, err
	}
	img := image.NewNRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	cc := new(colorCache)
	previous := color.NRGBA{A: 255}
	var x, y int
	for y < cfg.Height {
		c, err := decodeChunk(b)
		if err != nil {
			return nil, fmt.Errorf("decoding chunk starting at {x: %d, y: %d}: %w", x, y, err)
		}
		var n int
		n, previous = c.set(img, x, y, cc, previous)
		cc.add(previous)
		x, y = (x+n)%cfg.Width, y+(x+n)/cfg.Width
	}
	endMarker := make([]byte, 8)
	_, err = io.ReadFull(b, endMarker)
	if err != nil {
		return nil, fmt.Errorf("reading end marker: %w", err)
	}
	if !bytes.Equal(endMarker, []byte(endMarker)) {
		return nil, fmt.Errorf("bad end marker %x", endMarker)
	}
	return img, nil
}

type header struct {
	Magic         [4]byte
	Width, Height uint32
	Channels      uint8
	Colorspace    uint8
}

func Encode(w io.Writer, m image.Image) error {
	bw := bufio.NewWriter(w)
	b := m.Bounds()
	if b.Dx() > math.MaxUint32 || b.Dy() > math.MaxUint32 {
		return errors.New("image too large to encode")
	}
	hdr := header{
		Width:      uint32(b.Dx()),
		Height:     uint32(b.Dy()),
		Channels:   4,
		Colorspace: 0,
	}
	copy(hdr.Magic[:], magic)
	err := binary.Write(bw, binary.BigEndian, hdr)
	if err != nil {
		return err
	}
	cc := new(colorCache)
	previous := color.NRGBA{A: 255}
	r := newImageReader(m)
	for r.c != nil {
		seen := *r.c
		encodeChunk(bw, r, cc, previous)
		cc.add(seen)
		previous = seen
	}
	bw.Write([]byte(endMarker))
	return bw.Flush()
}

func encodeChunk(w *bufio.Writer, r *imageReader, cc *colorCache, previous color.NRGBA) {
	var op chunk
	if op = newOpRun(r, previous); op != nil {
	} else if op = newOpIndex(r, cc); op != nil {
	} else if op = newOpDiff(r, previous); op != nil {
	} else if op = newOpLuma(r, previous); op != nil {
	} else if op = newOpRGB(r, previous); op != nil {
	} else {
		op = newOpRGBA(r)
	}
	op.encode(w)
}

type imageReader struct {
	c *color.NRGBA
	p image.Point
	b image.Rectangle
	m image.Image
}

func newImageReader(m image.Image) *imageReader {
	b := m.Bounds()
	p := b.Min
	c := color.NRGBAModel.Convert(m.At(p.X, p.Y)).(color.NRGBA)
	return &imageReader{
		m: m,
		b: b,
		p: p,
		c: &c,
	}
}

func (r *imageReader) next() {
	r.p.X++
	if r.p.X == r.b.Max.X {
		r.p.X = r.b.Min.X
		r.p.Y++
	}
	if r.p.Y >= r.b.Max.Y {
		r.c = nil
		return
	}
	c := color.NRGBAModel.Convert(r.m.At(r.p.X, r.p.Y)).(color.NRGBA)
	r.c = &c
}

func DecodeConfig(r io.Reader) (cfg image.Config, err error) {
	var hdr header
	if err = binary.Read(r, binary.BigEndian, &hdr); err != nil {
		return
	}
	if !bytes.Equal(hdr.Magic[:], []byte(magic)) {
		return cfg, fmt.Errorf("expected magic = %q, got instead %q", magic, hdr.Magic[:])
	}
	return image.Config{
		Width:      int(hdr.Width),
		Height:     int(hdr.Height),
		ColorModel: color.NRGBAModel,
	}, err
}

type colorCache [64]color.NRGBA

func (cc *colorCache) add(c color.NRGBA) {
	idx := (c.R*3 + c.G*5 + c.B*7 + c.A*11) % 64
	cc[idx] = c
}

func (cc *colorCache) get(c color.NRGBA) int {
	idx := (c.R*3 + c.G*5 + c.B*7 + c.A*11) % 64
	if cc[idx] != c {
		return -1
	}
	return int(idx)
}

type chunk interface {
	decode(*bufio.Reader) error
	encode(*bufio.Writer)
	set(img *image.NRGBA, x, y int, cc *colorCache, previous color.NRGBA) (n int, c color.NRGBA)
}

type tag uint8

const (
	opTagRGB   tag = 0b11111110
	opTagRGBA  tag = 0b11111111
	opTagIndex tag = 0b00000000
	opTagDiff  tag = 0b01000000
	opTagLuma  tag = 0b10000000
	opTagRun   tag = 0b11000000
)

func decodeChunk(r *bufio.Reader) (chunk, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	_ = r.UnreadByte()
	var c chunk
	switch tag(b) {
	case opTagRGB:
		c = new(opRGB)
	case opTagRGBA:
		c = new(opRGBA)
	default:
		switch tag(b & 0b11000000) {
		case opTagIndex:
			c = new(opIndex)
		case opTagDiff:
			c = new(opDiff)
		case opTagLuma:
			c = new(opLuma)
		case opTagRun:
			c = new(opRun)
		}
	}
	return c, c.decode(r)
}
