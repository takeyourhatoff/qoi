package qoi

import (
	"bufio"
	"image"
	"image/color"
	"io"
)

type opRGB color.NRGBA

func newOpRGB(r *imageReader, previous color.NRGBA) chunk {
	if r.c.A != previous.A {
		return nil
	}
	defer r.next()
	return &opRGB{R: r.c.R, G: r.c.G, B: r.c.B}
}

func (op *opRGB) decode(r *bufio.Reader) error {
	var buf [4]uint8
	_, err := io.ReadFull(r, buf[:])
	*op = opRGB{R: buf[1], G: buf[2], B: buf[3]}
	return err
}

func (op opRGB) set(img *image.NRGBA, x, y int, cc *colorCache, previous color.NRGBA) (n int, c color.NRGBA) {
	op.A = previous.A
	return opRGBA(op).set(img, x, y, cc, previous)
}

func (op opRGB) encode(w *bufio.Writer) {
	w.Write([]byte{byte(opTagRGB), op.R, op.G, op.B})
}

type opRGBA color.NRGBA

func newOpRGBA(r *imageReader) chunk {
	defer r.next()
	return (*opRGBA)(r.c)
}

func (op *opRGBA) decode(r *bufio.Reader) error {
	var buf [5]uint8
	_, err := io.ReadFull(r, buf[:])
	*op = opRGBA{R: buf[1], G: buf[2], B: buf[3], A: buf[4]}
	return err
}

func (op opRGBA) set(img *image.NRGBA, x, y int, cc *colorCache, previous color.NRGBA) (n int, c color.NRGBA) {
	n = 1
	c = color.NRGBA(op)
	img.SetNRGBA(x, y, c)
	return
}

func (op opRGBA) encode(w *bufio.Writer) {
	w.Write([]byte{byte(opTagRGBA), op.R, op.G, op.B, op.A})
}

type opIndex uint8

func newOpIndex(r *imageReader, cc *colorCache) chunk {
	idx := cc.get(*r.c)
	if idx < 0 {
		return nil
	}
	r.next()
	op := opIndex(idx)
	return &op
}

func (op *opIndex) decode(r *bufio.Reader) error {
	buf, err := r.ReadByte()
	buf &= 0b00111111
	*op = opIndex(buf)
	return err
}

func (op opIndex) set(img *image.NRGBA, x, y int, cc *colorCache, previous color.NRGBA) (n int, c color.NRGBA) {
	n = 1
	c = cc[op]
	img.SetNRGBA(x, y, c)
	return
}

func (op opIndex) encode(w *bufio.Writer) {
	w.WriteByte(byte(opTagIndex) | byte(op))
}

type opDiff struct {
	dr, dg, db int8
}

func newOpDiff(r *imageReader, previous color.NRGBA) chunk {
	if r.c.A != previous.A {
		return nil
	}
	var op opDiff
	op.dr = int8(r.c.R - previous.R)
	op.dg = int8(r.c.G - previous.G)
	op.db = int8(r.c.B - previous.B)
	if op.dr < -2 || op.dr > 1 {
		return nil
	}
	if op.dg < -2 || op.dg > 1 {
		return nil
	}
	if op.db < -2 || op.db > 1 {
		return nil
	}
	r.next()
	return &op
}

func (op *opDiff) decode(r *bufio.Reader) error {
	buf, err := r.ReadByte()
	op.dr = int8((buf&0b00110000)>>4) - 2
	op.dg = int8((buf&0b00001100)>>2) - 2
	op.db = int8((buf&0b00000011)>>0) - 2
	return err
}

func (op opDiff) set(img *image.NRGBA, x, y int, cc *colorCache, previous color.NRGBA) (n int, c color.NRGBA) {
	n = 1
	c.R = uint8(int8(previous.R) + op.dr)
	c.G = uint8(int8(previous.G) + op.dg)
	c.B = uint8(int8(previous.B) + op.db)
	c.A = previous.A
	img.SetNRGBA(x, y, c)
	return
}

func (op opDiff) encode(w *bufio.Writer) {
	w.WriteByte(byte(opTagDiff) | (byte(op.dr+2) << 4) | (byte(op.dg+2) << 2) | byte(op.db+2))
}

type opLuma struct {
	dg   int8
	drdg int8
	dbdg int8
}

func newOpLuma(r *imageReader, previous color.NRGBA) chunk {
	if r.c.A != previous.A {
		return nil
	}
	var op opLuma
	op.dg = int8(r.c.G - previous.G)
	op.drdg = int8(r.c.R-previous.R) - op.dg
	op.dbdg = int8(r.c.B-previous.B) - op.dg
	if op.dg < -32 || op.dg > 31 {
		return nil
	}
	if op.drdg < -8 || op.drdg > 7 {
		return nil
	}
	if op.dbdg < -8 || op.dbdg > 7 {
		return nil
	}
	r.next()
	return &op
}

func (op *opLuma) decode(r *bufio.Reader) error {
	var buf [2]uint8
	_, err := io.ReadFull(r, buf[:])
	*op = opLuma{
		dg:   int8(buf[0]&0b00111111) - 32,
		drdg: int8(buf[1]&0b11110000>>4) - 8,
		dbdg: int8(buf[1]&0b00001111>>0) - 8,
	}
	return err
}

func (op opLuma) set(img *image.NRGBA, x, y int, cc *colorCache, previous color.NRGBA) (n int, c color.NRGBA) {
	n = 1
	c.R = uint8(int8(previous.R) + op.dg + op.drdg)
	c.G = uint8(int8(previous.G) + op.dg)
	c.B = uint8(int8(previous.B) + op.dg + op.dbdg)
	c.A = previous.A
	img.SetNRGBA(x, y, c)
	return
}

func (op opLuma) encode(w *bufio.Writer) {
	w.WriteByte(byte(opTagLuma) | byte(op.dg+32))
	w.WriteByte((byte(op.drdg+8) << 4) | byte(op.dbdg+8))
}

type opRun uint8

func newOpRun(r *imageReader, previous color.NRGBA) chunk {
	var run uint8
	for r.c != nil && *r.c == previous && run < 62 {
		run++
		r.next()
	}
	if run == 0 {
		return nil
	}
	return (*opRun)(&run)
}

func (op *opRun) decode(r *bufio.Reader) error {
	buf, err := r.ReadByte()
	buf &= 0b00111111
	buf++
	*op = opRun(buf)
	return err
}

func (op opRun) set(img *image.NRGBA, x, y int, cc *colorCache, previous color.NRGBA) (n int, c color.NRGBA) {
	n = int(op)
	c = previous
	for i := 1; i <= n; i++ {
		j := img.PixOffset(x, y)
		if j+4 > len(img.Pix) {
			return i, c
		}
		s := img.Pix[j : j+4 : j+4]
		s[0] = c.R
		s[1] = c.G
		s[2] = c.B
		s[3] = c.A
		x++
	}
	return
}

func (op opRun) encode(w *bufio.Writer) {
	w.WriteByte(byte(opTagRun) | byte(op-1))
}
