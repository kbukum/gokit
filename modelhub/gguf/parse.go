package gguf

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Source opens the first n bytes of a model file, such as with an HTTP range request.
type Source interface {
	OpenPrefix(ctx context.Context, n int64) (io.ReadCloser, error)
}

// Read opens a prefix of at most limits.MaxBytes from src, parses its header and closes the prefix.
func Read(ctx context.Context, src Source, limits Limits) (*Header, error) {
	if util.IsNil(src) {
		return nil, apperr.MissingField("source")
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	body, err := src.OpenPrefix(ctx, limits.MaxBytes)
	if err != nil {
		return nil, err
	}
	h, err := Parse(body, limits)
	// Parsing stops early by design, so closing abandons the rest of the prefix; a close failure cannot change the
	// facts already read.
	_ = body.Close()
	if err != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return h, err
}

// GGUF value types.
const (
	typeUint8 uint32 = iota
	typeInt8
	typeUint16
	typeInt16
	typeUint32
	typeInt32
	typeFloat32
	typeBool
	typeString
	typeArray
	typeUint64
	typeInt64
	typeFloat64
)

// maxKeyBytes is the specification's key length bound.
const maxKeyBytes = 65535

// errLimit stops parsing at a limit; it never escapes Parse.
type errLimit struct{ name string }

func (e errLimit) Error() string { return "gguf limit " + e.name }

// value is a decoded scalar or a kept integer or boolean array; elem is a kept array's element type.
type value struct {
	kind  uint32
	num   uint64
	neg   bool
	str   string
	array []uint64
	elem  uint32
	isInt bool
}

type parser struct {
	r      io.Reader
	left   int64
	order  binary.ByteOrder
	limits Limits
	buf    [8]byte
}

// Parse reads a GGUF header from r under limits. It never reads more than limits.MaxBytes.
func Parse(r io.Reader, limits Limits) (*Header, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, apperr.MissingField("reader")
	}
	p := &parser{r: r, left: limits.MaxBytes, order: binary.LittleEndian, limits: limits}
	h := &Header{}
	if err := p.frame(h); err != nil {
		return nil, frameError(err)
	}
	facts := map[string]value{}
	seen := map[string]struct{}{}
	for i := uint64(0); i < h.DeclaredEntries; i++ {
		if h.Entries >= limits.MaxEntries {
			h.Truncated, h.Limit = true, LimitEntries
			break
		}
		err := p.entry(seen, facts)
		var stop errLimit
		if errors.As(err, &stop) {
			h.Truncated, h.Limit = true, stop.name
			break
		}
		if err != nil {
			return nil, malformed(err)
		}
		h.Entries++
	}
	if err := h.apply(facts); err != nil {
		return nil, err
	}
	return h, nil
}

func (p *parser) frame(h *Header) error {
	magic, err := p.bytes(4)
	if err != nil {
		return err
	}
	if string(magic) != "GGUF" {
		return apperr.InvalidFormat("magic", "GGUF").WithReason(ReasonBadMagic)
	}
	raw, err := p.bytes(4)
	if err != nil {
		return err
	}
	version := binary.LittleEndian.Uint32(raw)
	// A big-endian file stores a small version that reads as a large little-endian number.
	if be := binary.BigEndian.Uint32(raw); version > 0xFFFF && be <= 0xFFFF {
		p.order, version, h.BigEndian = binary.BigEndian, be, true
	}
	if version != 2 && version != 3 {
		return apperr.InvalidFormat("version", "GGUF version 2 or 3").WithReason(ReasonUnsupportedVersion).WithDetail("version", version)
	}
	h.Version = version
	if h.TensorCount, err = p.u64(); err != nil {
		return err
	}
	h.DeclaredEntries, err = p.u64()
	return err
}

func (p *parser) entry(seen map[string]struct{}, facts map[string]value) error {
	key, err := p.key()
	if err != nil {
		return err
	}
	if _, dup := seen[key]; dup {
		return errors.New("duplicate key")
	}
	seen[key] = struct{}{}
	kind, err := p.u32()
	if err != nil {
		return err
	}
	if !wanted(key) {
		return p.skip(kind, 0)
	}
	v, err := p.value(kind, keepsArray(key))
	if err != nil {
		return err
	}
	facts[key] = v
	return nil
}

func (p *parser) key() (string, error) {
	n, err := p.u64()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", errors.New("empty key")
	}
	if n > maxKeyBytes || n > uint64(p.limits.MaxStringBytes) {
		return "", errLimit{LimitString}
	}
	raw, err := p.bytes(int64(n))
	if err != nil {
		return "", err
	}
	for _, c := range raw {
		if c <= ' ' || c >= 0x7F {
			return "", errors.New("key is not printable ASCII")
		}
	}
	return string(raw), nil
}

func (p *parser) value(kind uint32, keepArray bool) (value, error) {
	switch kind {
	case typeString:
		s, err := p.str()
		return value{kind: kind, str: s}, err
	case typeArray:
		if !keepArray {
			return value{kind: kind}, p.skip(kind, 0)
		}
		arr, elem, err := p.intArray()
		return value{kind: kind, array: arr, elem: elem}, err
	case typeBool:
		b, err := p.bytes(1)
		if err != nil {
			return value{}, err
		}
		if b[0] > 1 {
			return value{}, errors.New("invalid bool")
		}
		return value{kind: kind, num: uint64(b[0])}, nil
	default:
		num, neg, isInt, err := p.scalar(kind)
		return value{kind: kind, num: num, neg: neg, isInt: isInt}, err
	}
}

// scalar reads a numeric value; floats are consumed but report isInt=false.
func (p *parser) scalar(kind uint32) (num uint64, neg, isInt bool, err error) {
	size := scalarSize(kind)
	if size == 0 {
		return 0, false, false, errors.New("unknown value type")
	}
	raw, err := p.bytes(size)
	if err != nil {
		return 0, false, false, err
	}
	switch kind {
	case typeBool:
		if raw[0] > 1 {
			return 0, false, false, errors.New("invalid bool")
		}
		return uint64(raw[0]), false, true, nil
	case typeUint8:
		return uint64(raw[0]), false, true, nil
	case typeInt8:
		return signed(int64(int8(raw[0])))
	case typeUint16:
		return uint64(p.order.Uint16(raw)), false, true, nil
	case typeInt16:
		return signed(int64(int16(p.order.Uint16(raw))))
	case typeUint32:
		return uint64(p.order.Uint32(raw)), false, true, nil
	case typeInt32:
		return signed(int64(int32(p.order.Uint32(raw))))
	case typeUint64:
		return p.order.Uint64(raw), false, true, nil
	case typeInt64:
		return signed(int64(p.order.Uint64(raw)))
	default:
		return 0, false, false, nil
	}
}

func signed(v int64) (value uint64, negative, ok bool, err error) {
	if v < 0 {
		return 0, true, true, nil
	}
	return uint64(v), false, true, nil
}

func scalarSize(kind uint32) int64 {
	switch kind {
	case typeUint8, typeInt8, typeBool:
		return 1
	case typeUint16, typeInt16:
		return 2
	case typeUint32, typeInt32, typeFloat32:
		return 4
	case typeUint64, typeInt64, typeFloat64:
		return 8
	default:
		return 0
	}
}

func (p *parser) intArray() (values []uint64, elem uint32, err error) {
	elem, err = p.u32()
	if err != nil {
		return nil, 0, err
	}
	n, err := p.u64()
	if err != nil {
		return nil, 0, err
	}
	switch elem {
	case typeUint8, typeInt8, typeUint16, typeInt16, typeUint32, typeInt32, typeUint64, typeInt64, typeBool:
	default:
		return nil, 0, errors.New("kept array is not an integer or boolean array")
	}
	if n > uint64(p.limits.MaxArrayLen) {
		return nil, 0, errLimit{LimitArray}
	}
	if n*uint64(scalarSize(elem)) > uint64(p.left) {
		return nil, 0, errLimit{LimitBytes}
	}
	out := make([]uint64, 0, n)
	for range n {
		v, neg, _, err := p.scalar(elem)
		if err != nil {
			return nil, 0, err
		}
		if neg {
			return nil, 0, errors.New("negative count")
		}
		out = append(out, v)
	}
	return out, elem, nil
}

// skip consumes a value without keeping it.
func (p *parser) skip(kind uint32, depth int) error {
	switch kind {
	case typeString:
		n, err := p.u64()
		if err != nil {
			return err
		}
		if n > uint64(p.limits.MaxStringBytes) {
			return errLimit{LimitString}
		}
		return p.discard(int64(n))
	case typeArray:
		if depth >= p.limits.MaxDepth {
			return errLimit{LimitDepth}
		}
		elem, err := p.u32()
		if err != nil {
			return err
		}
		n, err := p.u64()
		if err != nil {
			return err
		}
		if size := scalarSize(elem); size > 0 {
			if elem == typeBool {
				return p.skipBools(n)
			}
			if n > uint64(p.left)/uint64(size) {
				return errLimit{LimitBytes}
			}
			return p.discard(int64(n) * size)
		}
		if elem != typeString && elem != typeArray {
			return errors.New("unknown array element type")
		}
		for range n {
			if err := p.skip(elem, depth+1); err != nil {
				return err
			}
		}
		return nil
	case typeBool:
		return p.skipBools(1)
	default:
		if size := scalarSize(kind); size > 0 {
			return p.discard(size)
		}
		return errors.New("unknown value type")
	}
}

func (p *parser) skipBools(n uint64) error {
	if n > uint64(p.left) {
		return errLimit{LimitBytes}
	}
	for range n {
		b, err := p.bytes(1)
		if err != nil {
			return err
		}
		if b[0] > 1 {
			return errors.New("invalid bool")
		}
	}
	return nil
}

func (p *parser) str() (string, error) {
	n, err := p.u64()
	if err != nil {
		return "", err
	}
	if n > uint64(p.limits.MaxStringBytes) {
		return "", errLimit{LimitString}
	}
	raw, err := p.bytes(int64(n))
	if err != nil {
		return "", err
	}
	s := string(raw)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	return s, nil
}

func (p *parser) u32() (uint32, error) {
	raw, err := p.bytes(4)
	if err != nil {
		return 0, err
	}
	return p.order.Uint32(raw), nil
}

func (p *parser) u64() (uint64, error) {
	raw, err := p.bytes(8)
	if err != nil {
		return 0, err
	}
	return p.order.Uint64(raw), nil
}

// bytes reads exactly n bytes within the budget. Short scalars reuse the parser buffer.
func (p *parser) bytes(n int64) ([]byte, error) {
	if n > p.left {
		return nil, errLimit{LimitBytes}
	}
	var out []byte
	if n <= int64(len(p.buf)) {
		out = p.buf[:n]
	} else {
		out = make([]byte, n)
	}
	read, err := io.ReadFull(p.r, out)
	p.left -= int64(read)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (p *parser) discard(n int64) error {
	if n > p.left {
		return errLimit{LimitBytes}
	}
	read, err := io.CopyN(io.Discard, p.r, n)
	p.left -= read
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

func frameError(err error) error {
	var app *apperr.AppError
	if errors.As(err, &app) {
		return app
	}
	return malformed(err)
}

func malformed(err error) error {
	e := apperr.InvalidFormat("gguf", "a GGUF header").WithReason(ReasonMalformed)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return e.WithCause(err)
	}
	return e.WithCause(io.ErrUnexpectedEOF)
}
