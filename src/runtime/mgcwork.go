// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/goarch"
	"runtime/internal/atomic"
	"unsafe"
)

const (
	_WorkbufSize = 2048 // in bytes; larger values result in less contention

	// workbufAlloc is the number of bytes to allocate at a time
	// for new workbufs. This must be a multiple of pageSize and
	// should be a multiple of _WorkbufSize.
	//
	// Larger values reduce workbuf allocation overhead. Smaller
	// values reduce heap fragmentation.
	workbufAlloc = 32 << 10
)

func init() {
	if workbufAlloc%pageSize != 0 || workbufAlloc%_WorkbufSize != 0 {
		throw("bad workbufAlloc")
	}
}

// Garbage collector work pool abstraction.
//
// This implements a producer/consumer model for pointers to grey
// objects. A grey object is one that is marked and on a work
// queue. A black object is marked and not on a work queue.
//
// Write barriers, root discovery, stack scanning, and object scanning
// produce pointers to grey objects. Scanning consumes pointers to
// grey objects, thus blackening them, and then scans them,
// potentially producing new pointers to grey objects.

// A gcWork provides the interface to produce and consume work for the
// garbage collector.
//
// A gcWork can be used on the stack as follows:
//
//     (preemption must be disabled)
//     gcw := &getg().m.p.ptr().gcw
//     .. call gcw.put() to produce and gcw.tryGet() to consume ..
//
// It's important that any use of gcWork during the mark phase prevent
// the garbage collector from transitioning to mark termination since
// gcWork may locally hold GC work buffers. This can be done by
// disabling preemption (systemstack or acquirem).
type gcWork struct {
	// 垃圾收集器提供了生产和消费任务的抽象，该结构体持有了两个重要的工作缓冲区 wbuf1 和 wbuf2，这两个缓冲区分别是主缓冲区和备缓冲区
	// 添加任务时总是从wbuf1添加，wbuf1满了就交换wbuf1和wbuf2，如果还是满的，就把当前wbuf1的工作flush到全局工作缓存中去。
	// wbuf1 and wbuf2 are the primary and secondary work buffers.
	//
	// This can be thought of as a stack of both work buffers'
	// pointers concatenated. When we pop the last pointer, we
	// shift the stack up by one work buffer by bringing in a new
	// full buffer and discarding an empty one. When we fill both
	// buffers, we shift the stack down by one work buffer by
	// bringing in a new empty buffer and discarding a full one.
	// This way we have one buffer's worth of hysteresis, which
	// amortizes the cost of getting or putting a work buffer over
	// at least one buffer of work and reduces contention on the
	// global work lists.
	//
	// wbuf1 is always the buffer we're currently pushing to and
	// popping from and wbuf2 is the buffer that will be discarded
	// next.
	//
	// Invariant: Both wbuf1 and wbuf2 are nil or neither are.
	wbuf1, wbuf2 *workbuf
	/**
	并发标记的分工问题？写屏障记录集的竞争问题？
	mark worker执行GC标记工作消耗工作队列时,会处理本地工作队列和全局工作缓存中工作量的均衡问题（runtime.gcDrain和runtime.gcDrainN中）。

	（1）如果全局工作缓存为空，就把当前p的工作分一些到全局工作队列中。具体做法是：如果wbuf2不为空，就把wbuf2整个flush到全局工作缓存中；如果wbuf2为空，wbuf1中元素个数大于4，就把wbuf1中一半的工作放到全局工作缓存中。

	（2）如果本地工作队列为空，就从全局工作缓存获取任务放到本地队列中。

	通过区分本地工作队列与全局工作缓存，缓解了执行并发标记工作时操作工作队列的竞争问题。

	而mutator触发写屏障时并不会直接操作工作队列，而是把相关指针写入当前p的写屏障缓冲区(p.wbBuf)中。
	当wbBuf已满或mark worker通过工作队列获取不到任务时,会把写屏障缓冲内容flush到工作缓存中,这样避免了mutator与GC之间关于写屏障记录的竞争问题。
	*/

	// Bytes marked (blackened) on this gcWork. This is aggregated
	// into work.bytesMarked by dispose.
	bytesMarked uint64

	// Heap scan work performed on this gcWork. This is aggregated into
	// gcController by dispose and may also be flushed by callers.
	// Other types of scan work are flushed immediately.
	heapScanWork int64

	// flushedWork indicates that a non-empty work buffer was
	// flushed to the global work list since the last gcMarkDone
	// termination check. Specifically, this indicates that this
	// gcWork may have communicated work to another gcWork.
	flushedWork bool
}

// Most of the methods of gcWork are go:nowritebarrierrec because the
// write barrier itself can invoke gcWork methods but the methods are
// not generally re-entrant. Hence, if a gcWork method invoked the
// write barrier while the gcWork was in an inconsistent state, and
// the write barrier in turn invoked a gcWork method, it could
// permanently corrupt the gcWork.

func (w *gcWork) init() {
	w.wbuf1 = getempty()
	wbuf2 := trygetfull()
	if wbuf2 == nil {
		wbuf2 = getempty()
	}
	w.wbuf2 = wbuf2
}

// put enqueues a pointer for the garbage collector to trace.
// obj must point to the beginning of a heap object or an oblet.
//go:nowritebarrierrec
func (w *gcWork) put(obj uintptr) {
	flushed := false
	wbuf := w.wbuf1
	// Record that this may acquire the wbufSpans or heap lock to
	// allocate a workbuf.
	lockWithRankMayAcquire(&work.wbufSpans.lock, lockRankWbufSpans)
	lockWithRankMayAcquire(&mheap_.lock, lockRankMheap)
	if wbuf == nil {
		w.init()
		wbuf = w.wbuf1
		// wbuf is empty at this point.
	} else if wbuf.nobj == len(wbuf.obj) {
		w.wbuf1, w.wbuf2 = w.wbuf2, w.wbuf1
		wbuf = w.wbuf1
		if wbuf.nobj == len(wbuf.obj) {
			putfull(wbuf)
			w.flushedWork = true
			wbuf = getempty()
			w.wbuf1 = wbuf
			flushed = true
		}
	}

	wbuf.obj[wbuf.nobj] = obj
	wbuf.nobj++

	// If we put a buffer on full, let the GC controller know so
	// it can encourage more workers to run. We delay this until
	// the end of put so that w is in a consistent state, since
	// enlistWorker may itself manipulate w.
	if flushed && gcphase == _GCmark {
		gcController.enlistWorker()
	}
}

// putFast does a put and reports whether it can be done quickly
// otherwise it returns false and the caller needs to call put.
//go:nowritebarrierrec
func (w *gcWork) putFast(obj uintptr) bool {
	wbuf := w.wbuf1
	if wbuf == nil {
		return false
	} else if wbuf.nobj == len(wbuf.obj) {
		return false
	}

	wbuf.obj[wbuf.nobj] = obj
	wbuf.nobj++
	return true
}

// putBatch performs a put on every pointer in obj. See put for
// constraints on these pointers.
//
//go:nowritebarrierrec
func (w *gcWork) putBatch(obj []uintptr) {
	if len(obj) == 0 {
		return
	}

	flushed := false
	wbuf := w.wbuf1
	if wbuf == nil {
		w.init()
		wbuf = w.wbuf1
	}

	for len(obj) > 0 {
		for wbuf.nobj == len(wbuf.obj) {
			putfull(wbuf)
			w.flushedWork = true
			w.wbuf1, w.wbuf2 = w.wbuf2, getempty()
			wbuf = w.wbuf1
			flushed = true
		}
		n := copy(wbuf.obj[wbuf.nobj:], obj)
		wbuf.nobj += n
		obj = obj[n:]
	}

	if flushed && gcphase == _GCmark {
		gcController.enlistWorker()
	}
}

// tryGet dequeues a pointer for the garbage collector to trace.
//
// If there are no pointers remaining in this gcWork or in the global
// queue, tryGet returns 0.  Note that there may still be pointers in
// other gcWork instances or other caches.
//go:nowritebarrierrec
func (w *gcWork) tryGet() uintptr {
	wbuf := w.wbuf1
	if wbuf == nil {
		w.init()
		wbuf = w.wbuf1
		// wbuf is empty at this point.
	}
	if wbuf.nobj == 0 {
		w.wbuf1, w.wbuf2 = w.wbuf2, w.wbuf1
		wbuf = w.wbuf1
		if wbuf.nobj == 0 {
			owbuf := wbuf
			wbuf = trygetfull()
			if wbuf == nil {
				return 0
			}
			putempty(owbuf)
			w.wbuf1 = wbuf
		}
	}

	wbuf.nobj--
	return wbuf.obj[wbuf.nobj]
}

// tryGetFast dequeues a pointer for the garbage collector to trace
// if one is readily available. Otherwise it returns 0 and
// the caller is expected to call tryGet().
//go:nowritebarrierrec
func (w *gcWork) tryGetFast() uintptr {
	wbuf := w.wbuf1
	if wbuf == nil {
		return 0
	}
	if wbuf.nobj == 0 {
		return 0
	}

	wbuf.nobj--
	return wbuf.obj[wbuf.nobj]
}

// dispose returns any cached pointers to the global queue.
// The buffers are being put on the full queue so that the
// write barriers will not simply reacquire them before the
// GC can inspect them. This helps reduce the mutator's
// ability to hide pointers during the concurrent mark phase.
//
//go:nowritebarrierrec
func (w *gcWork) dispose() {
	if wbuf := w.wbuf1; wbuf != nil {
		if wbuf.nobj == 0 {
			putempty(wbuf)
		} else {
			putfull(wbuf)
			w.flushedWork = true
		}
		w.wbuf1 = nil

		wbuf = w.wbuf2
		if wbuf.nobj == 0 {
			putempty(wbuf)
		} else {
			putfull(wbuf)
			w.flushedWork = true
		}
		w.wbuf2 = nil
	}
	if w.bytesMarked != 0 {
		// dispose happens relatively infrequently. If this
		// atomic becomes a problem, we should first try to
		// dispose less and if necessary aggregate in a per-P
		// counter.
		atomic.Xadd64(&work.bytesMarked, int64(w.bytesMarked))
		w.bytesMarked = 0
	}
	if w.heapScanWork != 0 {
		gcController.heapScanWork.Add(w.heapScanWork)
		w.heapScanWork = 0
	}
}

// balance moves some work that's cached in this gcWork back on the
// global queue.
//go:nowritebarrierrec
func (w *gcWork) balance() {
	// 当我们向该结构体中增加或者删除对象时，它总会先操作主缓冲区，
	// 一旦主缓冲区空间不足或者没有对象，会触发主备缓冲区的切换；而当两个缓冲区空间都不足或者都为空时，会从全局的工作缓冲区中插入或者获取对象
	if w.wbuf1 == nil {
		return
	}
	if wbuf := w.wbuf2; wbuf.nobj != 0 {
		putfull(wbuf)
		w.flushedWork = true
		w.wbuf2 = getempty()
	} else if wbuf := w.wbuf1; wbuf.nobj > 4 {
		w.wbuf1 = handoff(wbuf)
		w.flushedWork = true // handoff did putfull
	} else {
		return
	}
	// We flushed a buffer to the full list, so wake a worker.
	if gcphase == _GCmark {
		gcController.enlistWorker()
	}
}

// empty reports whether w has no mark work available.
//go:nowritebarrierrec
func (w *gcWork) empty() bool {
	return w.wbuf1 == nil || (w.wbuf1.nobj == 0 && w.wbuf2.nobj == 0)
}

// Internally, the GC work pool is kept in arrays in work buffers.
// The gcWork interface caches a work buffer until full (or empty) to
// avoid contending on the global work buffer lists.

type workbufhdr struct {
	node lfnode // must be first
	nobj int
}

//go:notinheap
type workbuf struct {
	workbufhdr
	// account for the above fields
	obj [(_WorkbufSize - unsafe.Sizeof(workbufhdr{})) / goarch.PtrSize]uintptr
}

// workbuf factory routines. These funcs are used to manage the
// workbufs.
// If the GC asks for some work these are the only routines that
// make wbufs available to the GC.

func (b *workbuf) checknonempty() {
	if b.nobj == 0 {
		throw("workbuf is empty")
	}
}

func (b *workbuf) checkempty() {
	if b.nobj != 0 {
		throw("workbuf is not empty")
	}
}

// getempty pops an empty work buffer off the work.empty list,
// allocating new buffers if none are available.
//go:nowritebarrier
func getempty() *workbuf {
	var b *workbuf
	if work.empty != 0 {
		b = (*workbuf)(work.empty.pop())
		if b != nil {
			b.checkempty()
		}
	}
	// Record that this may acquire the wbufSpans or heap lock to
	// allocate a workbuf.
	lockWithRankMayAcquire(&work.wbufSpans.lock, lockRankWbufSpans)
	lockWithRankMayAcquire(&mheap_.lock, lockRankMheap)
	if b == nil {
		// Allocate more workbufs.
		var s *mspan
		if work.wbufSpans.free.first != nil {
			lock(&work.wbufSpans.lock)
			s = work.wbufSpans.free.first
			if s != nil {
				work.wbufSpans.free.remove(s)
				work.wbufSpans.busy.insert(s)
			}
			unlock(&work.wbufSpans.lock)
		}
		if s == nil {
			systemstack(func() {
				s = mheap_.allocManual(workbufAlloc/pageSize, spanAllocWorkBuf)
			})
			if s == nil {
				throw("out of memory")
			}
			// Record the new span in the busy list.
			lock(&work.wbufSpans.lock)
			work.wbufSpans.busy.insert(s)
			unlock(&work.wbufSpans.lock)
		}
		// Slice up the span into new workbufs. Return one and
		// put the rest on the empty list.
		for i := uintptr(0); i+_WorkbufSize <= workbufAlloc; i += _WorkbufSize {
			newb := (*workbuf)(unsafe.Pointer(s.base() + i))
			newb.nobj = 0
			lfnodeValidate(&newb.node)
			if i == 0 {
				b = newb
			} else {
				putempty(newb)
			}
		}
	}
	return b
}

// putempty puts a workbuf onto the work.empty list.
// Upon entry this goroutine owns b. The lfstack.push relinquishes ownership.
//go:nowritebarrier
func putempty(b *workbuf) {
	b.checkempty()
	work.empty.push(&b.node)
}

// putfull puts the workbuf on the work.full list for the GC.
// putfull accepts partially full buffers so the GC can avoid competing
// with the mutators for ownership of partially full buffers.
//go:nowritebarrier
func putfull(b *workbuf) {
	b.checknonempty()
	work.full.push(&b.node)
}

// trygetfull tries to get a full or partially empty workbuffer.
// If one is not immediately available return nil
//go:nowritebarrier
func trygetfull() *workbuf {
	b := (*workbuf)(work.full.pop())
	if b != nil {
		b.checknonempty()
		return b
	}
	return b
}

//go:nowritebarrier
func handoff(b *workbuf) *workbuf {
	// Make new buffer with half of b's pointers.
	b1 := getempty()
	n := b.nobj / 2
	b.nobj -= n
	b1.nobj = n
	memmove(unsafe.Pointer(&b1.obj[0]), unsafe.Pointer(&b.obj[b.nobj]), uintptr(n)*unsafe.Sizeof(b1.obj[0]))

	// Put b on full list - let first half of b get stolen.
	putfull(b)
	return b1
}

// prepareFreeWorkbufs moves busy workbuf spans to free list so they
// can be freed to the heap. This must only be called when all
// workbufs are on the empty list.
func prepareFreeWorkbufs() {
	lock(&work.wbufSpans.lock)
	if work.full != 0 {
		throw("cannot free workbufs when work.full != 0")
	}
	// Since all workbufs are on the empty list, we don't care
	// which ones are in which spans. We can wipe the entire empty
	// list and move all workbuf spans to the free list.
	work.empty = 0
	work.wbufSpans.free.takeAll(&work.wbufSpans.busy)
	unlock(&work.wbufSpans.lock)
}

// freeSomeWbufs frees some workbufs back to the heap and returns
// true if it should be called again to free more.
// freeSomeWbufs 释放一些 workbufs 回到堆中，如果需要再次调用则返回 true
func freeSomeWbufs(preemptible bool) bool {
	const batchSize = 64 // ~1–2 µs per span.
	lock(&work.wbufSpans.lock)
	// 如果此时在标记阶段、或者 wbufSpans 为空，则不需要进行释放
	// 因为标记阶段 workbufs 需要被标记，而 workbufs 为空则更不需要释放
	if gcphase != _GCoff || work.wbufSpans.free.isEmpty() {
		unlock(&work.wbufSpans.lock)
		return false
	}
	systemstack(func() {
		gp := getg().m.curg
		// 清扫一批 span，64 个，大约 ~1–2 µs
		// 在需要被抢占时停止、在清扫完毕后停止
		for i := 0; i < batchSize && !(preemptible && gp.preempt); i++ {
			span := work.wbufSpans.free.first
			if span == nil {
				break
			}
			// 将 span 移除 wbufSpans 的空闲链表中
			work.wbufSpans.free.remove(span)
			// 将 span 归还到 mheap 中
			mheap_.freeManual(span, spanAllocWorkBuf)
		}
	})
	// workbufs 的空闲 span 列表尚未清空，还需要更多清扫
	more := !work.wbufSpans.free.isEmpty()
	unlock(&work.wbufSpans.lock)
	return more
}
