// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"time"

	"github.com/syndtr/goleveldb/leveldb/memdb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func (d *DB) doWriteJournal(b *Batch) error {
	err := d.journal.journal.Append(b.encode())
	if err == nil && b.sync {
		err = d.journal.writer.Sync()
	}
	return err
}

func (d *DB) writeJournal() {
	for b := range d.jch {
		if b == nil {
			break
		}

		// write journal
		d.jack <- d.doWriteJournal(b)
	}

	close(d.jch)
	close(d.jack)
	d.ewg.Done()
}

func (d *DB) flush() (m *memdb.DB, err error) {
	s := d.s

	delayed, cwait := false, false
	for {
		v := s.version()
		mem, frozenMem := d.getMem()
		switch {
		case v.tLen(0) >= kL0_SlowdownWritesTrigger && !delayed:
			delayed = true
			time.Sleep(time.Millisecond)
		case mem.Size() <= s.o.GetWriteBuffer():
			// still room
			v.release()
			return mem, nil
		case frozenMem != nil:
			if cwait {
				err = d.geterr()
				if err != nil {
					return
				}
				d.cch <- cSched
			} else {
				cwait = true
				d.cch <- cWait
			}
		case v.tLen(0) >= kL0_StopWritesTrigger:
			d.cch <- cSched
		default:
			// create new memdb and journal
			m, err = d.newMem()
			if err != nil {
				return
			}

			// schedule compaction
			select {
			case d.cch <- cSched:
			default:
			}
		}
		v.release()
	}

	return
}

// Write apply the given batch to the DB. The batch will be applied
// sequentially.
//
// It is safe to modify the contents of the arguments after Write returns.
func (d *DB) Write(b *Batch, wo *opt.WriteOptions) (err error) {
	err = d.wok()
	if err != nil || b == nil || b.len() == 0 {
		return
	}

	b.init(wo.HasFlag(opt.WFSync))

	// The write happen synchronously.
	select {
	case d.wqueue <- b:
		return <-d.wack
	case d.wlock <- struct{}{}:
	}

	merged := 0
	defer func() {
		<-d.wlock
		for i := 0; i < merged; i++ {
			d.wack <- err
		}
	}()

	mem, err := d.flush()
	if err != nil {
		return
	}

	// calculate maximum size of the batch
	m := 1 << 20
	if x := b.size(); x <= 128<<10 {
		m = x + (128 << 10)
	}

	// merge with other batch
drain:
	for b.size() <= m && !b.sync {
		select {
		case nb := <-d.wqueue:
			b.append(nb)
			merged++
		default:
			break drain
		}
	}

	// set batch first seq number relative from last seq
	b.seq = d.seq + 1

	// write journal concurrently if it is large enough
	if b.size() >= (128 << 10) {
		d.jch <- b
		b.memReplay(mem)
		err = <-d.jack
		if err != nil {
			b.revertMemReplay(mem)
			return
		}
	} else {
		err = d.doWriteJournal(b)
		if err != nil {
			return
		}
		b.memReplay(mem)
	}

	// set last seq number
	d.addSeq(uint64(b.len()))

	return
}

// Put sets the value for the given key. It overwrites any previous value
// for that key; a DB is not a multi-map.
//
// It is safe to modify the contents of the arguments after Put returns.
func (d *DB) Put(key, value []byte, wo *opt.WriteOptions) error {
	b := new(Batch)
	b.Put(key, value)
	return d.Write(b, wo)
}

// Delete deletes the value for the given key. It returns errors.ErrNotFound if
// the DB does not contain the key.
//
// It is safe to modify the contents of the arguments after Delete returns.
func (d *DB) Delete(key []byte, wo *opt.WriteOptions) error {
	b := new(Batch)
	b.Delete(key)
	return d.Write(b, wo)
}
