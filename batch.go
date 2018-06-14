// Copyright 2012 The LevelDB-Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pebble

import (
	"encoding/binary"

	"github.com/petermattis/pebble/db"
)

const batchHeaderLen = 12

const invalidBatchCount = 1<<32 - 1

// Batch is a sequence of Sets and/or Deletes that are applied atomically.
type Batch struct {
	// The parent writer to which the batch will be committed.
	parent db.Writer
	// Data is the wire format of a batch's log entry:
	//   - 8 bytes for a sequence number of the first batch element,
	//     or zeroes if the batch has not yet been applied,
	//   - 4 bytes for the count: the number of elements in the batch,
	//     or "\xff\xff\xff\xff" if the batch is invalid,
	//   - count elements, being:
	//     - one byte for the kind: delete (0) or set (1),
	//     - the varint-string user key,
	//     - the varint-string value (if kind == set).
	// The sequence number and count are stored in little-endian order.
	data []byte

	// TODO(peter): Add an optional skiplist keyed by offset into data of the
	// entry. If the Batch is configured to track mutations we index the entries
	// such that they can be iterated over in key (and descending insertion)
	// order.
}

// Batch implements the db.Reader interface.
var _ db.Reader = (*Batch)(nil)

// Apply implements DB.Apply, as documented in the pebble/db package.
func (b *Batch) Apply(batch []byte) error {
	panic("pebble.Batch: Apply unimplemented")
}

// Get implements DB.Get, as documented in the pebble/db package.
func (b *Batch) Get(key []byte, o *db.ReadOptions) (value []byte, err error) {
	panic("pebble.Batch: Get unimplemented")
}

// Set adds an action to the batch that sets the key to map to the value.
func (b *Batch) Set(key, value []byte) {
	if len(b.data) == 0 {
		b.init(len(key) + len(value) + 2*binary.MaxVarintLen64 + batchHeaderLen)
	}
	if b.increment() {
		b.data = append(b.data, byte(internalKeyKindSet))
		b.appendStr(key)
		b.appendStr(value)
	}
}

// Merge adds an action to the batch that merges the value at key with the new
// value. The details of the merge are dependent upon the configured merge
// operator.
func (b *Batch) Merge(key, value []byte) {
	panic("pebble.Batch: Merge unimplemented")
}

// Delete adds an action to the batch that deletes the entry for key.
func (b *Batch) Delete(key []byte) {
	if len(b.data) == 0 {
		b.init(len(key) + binary.MaxVarintLen64 + batchHeaderLen)
	}
	if b.increment() {
		b.data = append(b.data, byte(internalKeyKindDelete))
		b.appendStr(key)
	}
}

// DeleteRange implements DB.DeleteRange, as documented in the pebble/db
// package.
func (b *Batch) DeleteRange(start, end []byte) {
	panic("pebble.Batch: DeleteRange unimplemented")
}

// Find implements DB.Find, as documented in the pebble/db package.
func (b *Batch) Find(key []byte, o *db.ReadOptions) db.Iterator {
	panic("pebble.Batch: Findunimplemented")
}

// NewIter implements DB.NewIter, as documented in the pebble/db package.
func (b *Batch) NewIter(o *db.ReadOptions) db.Iterator {
	panic("pebble.Batch: NewIter unimplemented")
}

// Commit applies the batch to its parent writer.
func (b *Batch) Commit(o *db.WriteOptions) error {
	panic("pebble.Batch: Commit unimplemented")
}

// Close implements DB.Close, as documented in the pebble/db package.
func (b *Batch) Close() error {
	return nil
}

func (b *Batch) init(cap int) {
	n := 256
	for n < cap {
		n *= 2
	}
	b.data = make([]byte, batchHeaderLen, n)
}

// seqNumData returns the 8 byte little-endian sequence number. Zero means that
// the batch has not yet been applied.
func (b *Batch) seqNumData() []byte {
	return b.data[:8]
}

// countData returns the 4 byte little-endian count data. "\xff\xff\xff\xff"
// means that the batch is invalid.
func (b *Batch) countData() []byte {
	return b.data[8:12]
}

func (b *Batch) increment() (ok bool) {
	p := b.countData()
	for i := range p {
		p[i]++
		if p[i] != 0x00 {
			return true
		}
	}
	// The countData was "\xff\xff\xff\xff". Leave it as it was.
	p[0] = 0xff
	p[1] = 0xff
	p[2] = 0xff
	p[3] = 0xff
	return false
}

func (b *Batch) appendStr(s []byte) {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], uint64(len(s)))
	b.data = append(b.data, buf[:n]...)
	b.data = append(b.data, s...)
}

func (b *Batch) setSeqNum(seqNum uint64) {
	binary.LittleEndian.PutUint64(b.seqNumData(), seqNum)
}

func (b *Batch) seqNum() uint64 {
	return binary.LittleEndian.Uint64(b.seqNumData())
}

func (b *Batch) count() uint32 {
	return binary.LittleEndian.Uint32(b.countData())
}

func (b *Batch) iter() batchIter {
	return b.data[batchHeaderLen:]
}

type batchIter []byte

// next returns the next operation in this batch.
// The final return value is false if the batch is corrupt.
func (t *batchIter) next() (kind internalKeyKind, ukey []byte, value []byte, ok bool) {
	p := *t
	if len(p) == 0 {
		return 0, nil, nil, false
	}
	kind, *t = internalKeyKind(p[0]), p[1:]
	if kind > internalKeyKindMax {
		return 0, nil, nil, false
	}
	ukey, ok = t.nextStr()
	if !ok {
		return 0, nil, nil, false
	}
	if kind != internalKeyKindDelete {
		value, ok = t.nextStr()
		if !ok {
			return 0, nil, nil, false
		}
	}
	return kind, ukey, value, true
}

func (t *batchIter) nextStr() (s []byte, ok bool) {
	p := *t
	u, numBytes := binary.Uvarint(p)
	if numBytes <= 0 {
		return nil, false
	}
	p = p[numBytes:]
	if u > uint64(len(p)) {
		return nil, false
	}
	s, *t = p[:u], p[u:]
	return s, true
}
