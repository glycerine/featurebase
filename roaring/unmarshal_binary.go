// Copyright 2019 Pilosa Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package roaring

import (
	"errors"
	"io"
	"unsafe"
)

// UnmarshalBinary reads Pilosa's format, or upstream roaring (mostly;
// it may not handle some edge cases), and decodes them into the given
// bitmap, replacing the existing contents.
func (b *Bitmap) UnmarshalBinary(data []byte) (err error) {
	if data == nil {
		return errors.New("no roaring bitmap provided")
	}
	var itr roaringIterator
	var itrKey uint64
	var itrCType byte
	var itrN int
	var itrLen int
	var itrPointer *uint16
	var itrErr error

	itr, err = newRoaringIterator(data)
	if err != nil {
		return err
	}
	if itr == nil {
		return errors.New("failed to create roaring iterator, but don't know why")
	}

	b.Containers.Reset()

	itrKey, itrCType, itrN, itrLen, itrPointer, itrErr = itr.Next()
	for itrErr == nil {
		var newC *Container
		switch itrCType {
		case containerArray:
			newC = NewContainerArray((*[4096]uint16)(unsafe.Pointer(itrPointer))[:itrLen:itrLen])
		case containerRun:
			newC = NewContainerRunN((*[2048]interval16)(unsafe.Pointer(itrPointer))[:itrLen:itrLen], int32(itrN))
		case containerBitmap:
			newC = NewContainerBitmapN((*[1024]uint64)(unsafe.Pointer(itrPointer))[:1024:itrLen], int32(itrN))
		default:
			panic("invalid container type")
		}
		newC.setMapped(true)
		if !b.preferMapping {
			newC.unmapOrClone()
		}
		b.Containers.Put(itrKey, newC)
		itrKey, itrCType, itrN, itrLen, itrPointer, itrErr = itr.Next()
	}
	// note: if we get a non-EOF err, it's possible that we made SOME
	// changes but didn't log them. I don't have a good solution to this.
	if itrErr != io.EOF {
		return itrErr
	}

	// Read ops log until the end of the file.
	b.ops = 0
	b.opN = 0
	buf, lastValidOffset := itr.Remaining()
	for {
		// Exit when there are no more ops to parse.
		if len(buf) == 0 {
			break
		}

		// Unmarshal the op and apply it.
		var opr op
		if err := opr.UnmarshalBinary(buf); err != nil {
			return newFileShouldBeTruncatedError(err, int64(lastValidOffset))
		}

		opr.apply(b)

		// Increase the op count.
		b.ops++
		b.opN += opr.count()

		// Move the buffer forward.
		opSize := opr.size()
		buf = buf[opSize:]
		lastValidOffset += int64(opSize)
	}
	return nil
}

// InspectBinary reads a roaring bitmap, plus a possible ops log,
// and reports back on the contents, including distinguishing between
// the original ops log and the post-ops-log contents.
func InspectBinary(data []byte, mapped bool, info *BitmapInfo) (b *Bitmap, mappedAny bool, err error) {
	b = NewFileBitmap()
	b.PreferMapping(mapped)
	if data == nil {
		return b, mappedAny, errors.New("no roaring bitmap provided")
	}
	var itr roaringIterator
	var itrKey uint64
	var itrCType byte
	var itrN int
	var itrLen int
	var itrPointer *uint16
	var itrErr error

	itr, err = newRoaringIterator(data)
	if err != nil {
		return b, mappedAny, err
	}
	if itr == nil {
		return b, mappedAny, errors.New("failed to create roaring iterator, but don't know why")
	}
	keys := itr.Len()
	info.Containers = make([]ContainerInfo, 0, keys)

	itrKey, itrCType, itrN, itrLen, itrPointer, itrErr = itr.Next()
	for itrErr == nil {
		var size int
		switch itrCType {
		case containerArray:
			size = int(itrN) * 2
		case containerBitmap:
			size = 8192
		case containerRun:
			size = itrLen*interval16Size + runCountHeaderSize
		}
		var newC *Container
		switch itrCType {
		case containerArray:
			newC = NewContainerArray((*[4096]uint16)(unsafe.Pointer(itrPointer))[:itrLen:itrLen])
		case containerRun:
			newC = NewContainerRunN((*[2048]interval16)(unsafe.Pointer(itrPointer))[:itrLen:itrLen], int32(itrN))
		case containerBitmap:
			newC = NewContainerBitmapN((*[1024]uint64)(unsafe.Pointer(itrPointer))[:1024:itrLen], int32(itrN))
		default:
			panic("invalid container type")
		}
		newC.setMapped(true)
		if !mapped {
			newC.unmapOrClone()
		}
		newC.flags |= flagPristine
		if newC.flags&flagMapped != 0 {
			mappedAny = true
		}
		info.Containers = append(info.Containers, ContainerInfo{
			N:       newC.n,
			Mapped:  newC.flags&flagMapped != 0,
			Type:    containerTypeNames[itrCType],
			Alloc:   size,
			Pointer: uintptr(unsafe.Pointer(newC.pointer)),
			Key:     itrKey,
			Flags:   newC.flags.String(),
		})
		info.ContainerCount++
		info.BitCount += uint64(newC.n)
		itrKey, itrCType, itrN, itrLen, itrPointer, itrErr = itr.Next()
	}
	// note: if we get a non-EOF err, it's possible that we made SOME
	// changes but didn't log them. I don't have a good solution to this.
	if itrErr != io.EOF {
		return b, mappedAny, itrErr
	}
	// stash pointer ranges
	info.From = uintptr(unsafe.Pointer(&data[0]))
	info.To = info.From + uintptr(len(data))

	// Read ops log until the end of the file.
	b.ops = 0
	b.opN = 0
	buf, lastValidOffset := itr.Remaining()
	// if there's no ops log, we're done and can just return the
	// info so far.
	if len(buf) == 0 {
		return b, mappedAny, err
	}
	for {
		// Exit when there are no more ops to parse.
		if len(buf) == 0 {
			break
		}

		// Unmarshal the op and apply it.
		var opr op
		if err = opr.UnmarshalBinary(buf); err != nil {
			// we break out here, but we continue on to
			// return the bitmap as-is, along with data about
			// it, and the error. this lets us share the
			// "is anything mapped" check with that code.
			break
		}
		opr.apply(b)

		// Increase the op count.
		if info != nil {
			info.Ops++
			info.OpN += opr.count()
			info.OpDetails = append(info.OpDetails, opr.info())
		}
		// Move the buffer forward.
		opSize := opr.size()
		buf = buf[opSize:]
		lastValidOffset += int64(opSize)
	}
	citer, _ := b.Containers.Iterator(0)
	// it's possible the ops log unmapped every mapped container, so we recheck.
	mappedAny = false
	if info == nil {
		for citer.Next() {
			_, c := citer.Value()
			if c.Mapped() {
				mappedAny = true
				break
			}
		}
		return b, mappedAny, err
	}
	// now we want to compute the actual container and bit counts after
	// ops, and create a report of just the containers which got changed.
	info.ContainerCount = 0
	info.BitCount = 0
	for citer.Next() {
		k, c := citer.Value()
		if c.Mapped() {
			mappedAny = true
		}
		info.ContainerCount++
		info.BitCount += uint64(c.N())
		if c.flags&flagPristine != 0 {
			continue
		}
		ci := c.info()
		ci.Key = k
		info.OpContainers = append(info.OpContainers, ci)
	}
	return b, mappedAny, err
}
