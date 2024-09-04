package udsipc

import (
	"bytes"
	"testing"
)

type AP struct {
}

// FromBuffer implements IMessage.
func (a *AP) FromBuffer(buf *bytes.Buffer) error {
	panic("unimplemented")
}

// ToBuffer implements IMessage.
func (a *AP) ToBuffer(buf *bytes.Buffer) error {
	panic("unimplemented")
}

var _ IMessage = &AP{}

type AV struct {
}

// FromBuffer implements IMessage.
func (a AV) FromBuffer(buf *bytes.Buffer) error {
	panic("unimplemented")
}

// ToBuffer implements IMessage.
func (a AV) ToBuffer(buf *bytes.Buffer) error {
	panic("unimplemented")
}

var _ IMessage = AV{}

type BV string

// FromBuffer implements IMessage.
func (b BV) FromBuffer(buf *bytes.Buffer) error {
	panic("unimplemented")
}

// ToBuffer implements IMessage.
func (b BV) ToBuffer(buf *bytes.Buffer) error {
	panic("unimplemented")
}

var _ IMessage = BV("")

func TestTypeName(t *testing.T) {

}
