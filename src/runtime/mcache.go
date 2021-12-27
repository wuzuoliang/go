// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"runtime/internal/atomic"
	"unsafe"
)

// Per-thread (in Go, per-P) cache for small objects.
// This includes a small object cache and local allocation stats.
// No locking needed because it is per-thread (per-P).
//
// mcaches are allocated from non-GC'd memory, so any heap pointers
// must be specially handled.
// runtime.mcache 是 Go 语言中的线程缓存，它会与线程上的处理器一一绑定，主要用来缓存用户程序申请的微小对象。
// 每一个线程缓存都持有 68 * 2 个 runtime.mspan，这些内存管理单元都存储在结构体的 alloc 字段中：
//go:notinheap
type mcache struct {
	// The following members are accessed on every malloc,
	// so they are grouped here for better caching.
	// 下面的成员在每次 malloc 时都会被访问
	// 因此将它们放到一起来利用缓存的局部性原理
	nextSample uintptr // trigger heap sample after allocating this many bytes 分配多少大小的堆时触发堆采样
	scanAlloc  uintptr // bytes of scannable heap allocated 分配的可扫描堆字节数

	// Allocator cache for tiny objects w/o pointers.
	// See "Tiny allocator" comment in malloc.go.

	// tiny points to the beginning of the current tiny block, or
	// nil if there is no current tiny block.
	//
	// tiny is a heap pointer. Since mcache is in non-GC'd memory,
	// we handle it by clearing it in releaseAll during mark
	// termination.
	// 没有指针的微小对象的分配器缓存。
	// 请参考 malloc.go 中的 "小型分配器" 注释。
	//
	// tiny 指向当前 tiny 块的起始位置，或当没有 tiny 块时候为 nil
	// tiny 是一个堆指针。由于 mcache 在非 GC 内存中，我们通过在
	// mark termination 期间在 releaseAll 中清除它来处理它
	//
	// tinyAllocs is the number of tiny allocations performed
	// by the P that owns this mcache.
	// 线程缓存中还包含几个用于分配微对象的字段,专门管理 16 字节以下的对象
	// 微分配器只会用于分配非指针类型的内存，上述三个字段中 tiny 会指向堆中的一片内存，tinyOffset 是下一个空闲内存所在的偏移量，最后的 local_tinyallocs 会记录内存分配器中分配的对象个数
	tiny       uintptr // 堆指针，指向当前 tiny 块的起始指针，如果当前无tiny块则为nil。在终止标记期间，通过调用 mcache.releaseAll() 来清除它
	tinyoffset uintptr // 当前tiny 块中所占线性位置偏移量
	tinyAllocs uintptr // 拥有当前 mcache 的 P 执行的微小分配数

	// The rest is not accessed on every malloc.

	// mcache.alloc 是一个数组，值为 *spans 类型，它是 go 中管理内存的基本单元。对于16-32 kb大小的内存都会使用这个数组里的的 spans 中分配。每个span存在两次，一个不包含指针的对象列表和另一个包含指针的对象列表。这种区别将使垃圾收集的工作更容易，因为它不必扫描不包含任何指针的范围。
	alloc [numSpanClasses]*mspan // spans to allocate from, indexed by spanClass 用来分配的 spans，由 spanClass 索引
	// 线程缓存在刚刚被初始化时是不包含 runtime.mspan 的，只有当用户程序申请内存时才会从上一级组件获取新的 runtime.mspan 满足内存分配的需求

	// 从调度器和内存分配的经验来看，如果运行时只使用全局变量来分配内存的话，
	// 势必会造成线程之间的锁竞争进而影响程序的执行效率，栈内存由于与线程关系比较密切，
	// 所以在每一个线程缓存 runtime.mcache 中都加入了栈缓存减少锁竞争影响
	stackcache [_NumStackOrders]stackfreelist

	// flushGen indicates the sweepgen during which this mcache
	// was last flushed. If flushGen != mheap_.sweepgen, the spans
	// in this mcache are stale and need to the flushed so they
	// can be swept. This is done in acquirep.
	flushGen uint32 // 表示上次刷新 mcache 的 sweepgen（清扫生成）。如果 flushGen != mheap_.sweepgen 则说明 mcache 已过期需要刷新，需被被清扫。在 acrequirep 中完成
}

// A gclink is a node in a linked list of blocks, like mlink,
// but it is opaque to the garbage collector.
// The GC does not trace the pointers during collection,
// and the compiler does not emit write barriers for assignments
// of gclinkptr values. Code should store references to gclinks
// as gclinkptr, not as *gclink.
type gclink struct {
	next gclinkptr
}

// A gclinkptr is a pointer to a gclink, but it is opaque
// to the garbage collector.
type gclinkptr uintptr

// ptr returns the *gclink form of p.
// The result should be used for accessing fields, not stored
// in other data structures.
func (p gclinkptr) ptr() *gclink {
	return (*gclink)(unsafe.Pointer(p))
}

type stackfreelist struct {
	list gclinkptr // linked list of free stacks
	size uintptr   // total size of stacks in list
}

// dummy mspan that contains no free objects.
var emptymspan mspan // 虚拟的MSpan，不包含任何对象。

// allocmcache 初始化线程缓存
// 运行时的 runtime.allocmcache 从 mheap 上分配一个 mcache。 由于 mheap 是全局的，因此在分配期必须对其进行加锁，而分配通过 fixAlloc 组件完成
func allocmcache() *mcache {
	var c *mcache
	systemstack(func() {
		lock(&mheap_.lock)
		c = (*mcache)(mheap_.cachealloc.alloc())
		c.flushGen = mheap_.sweepgen
		unlock(&mheap_.lock)
	})
	for i := range c.alloc {
		// 初始化后的 runtime.mcache 中的所有 runtime.mspan 都是空的占位符 emptymspan。
		// 暂时指向虚拟的 mspan 中
		c.alloc[i] = &emptymspan
	}
	// 返回下一个采样点，是服从泊松过程的随机数
	c.nextSample = nextSample()
	return c
}

// freemcache releases resources associated with this
// mcache and puts the object onto a free list.
//
// In some cases there is no way to simply release
// resources, such as statistics, so donate them to
// a different mcache (the recipient).
func freemcache(c *mcache) {
	systemstack(func() {
		// 归还 span
		c.releaseAll()
		// 释放 stack
		stackcache_clear(c)

		// NOTE(rsc,rlh): If gcworkbuffree comes back, we need to coordinate
		// with the stealing of gcworkbufs during garbage collection to avoid
		// a race where the workbuf is double-freed.
		// gcworkbuffree(c.gcworkbuf)

		lock(&mheap_.lock)
		// 将 mcache 释放
		mheap_.cachealloc.free(unsafe.Pointer(c))
		unlock(&mheap_.lock)
	})
}

// getMCache is a convenience function which tries to obtain an mcache.
//
// Returns nil if we're not bootstrapping or we don't have a P. The caller's
// P must not change, so we must be in a non-preemptible state.
func getMCache(mp *m) *mcache {
	// Grab the mcache, since that's where stats live.
	pp := mp.p.ptr()
	var c *mcache
	if pp == nil {
		// We will be called without a P while bootstrapping,
		// in which case we use mcache0, which is set in mallocinit.
		// mcache0 is cleared when bootstrapping is complete,
		// by procresize.
		// 在引导时，我们将在没有P的情况下被调用，在这种情况下，我们使用在mallocinit中设置的mcache0。当引导完成时，通过procresize清除McAche0。
		c = mcache0
	} else {
		c = pp.mcache
	}
	return c
}

// refill acquires a new span of span class spc for c. This span will
// have at least one free object. The current span in c must be full.
//
// Must run in a non-preemptible context since otherwise the owner of
// c could change.
// 会为线程缓存获取一个指定跨度类的内存管理单元，被替换的单元不能包含空闲的内存空间，而获取的单元中需要至少包含一个空闲对象用于分配内存
func (c *mcache) refill(spc spanClass) {
	// Return the current cached span to the central lists.
	// refill 其实是从 mcentral 调用 cacheSpan 方法来获得 span
	s := c.alloc[spc]

	if uintptr(s.allocCount) != s.nelems {
		throw("refill of span with free space remaining")
	}
	if s != &emptymspan {
		// Mark this span as no longer cached.
		if s.sweepgen != mheap_.sweepgen+3 {
			throw("bad sweepgen in refill")
		}
		mheap_.central[spc].mcentral.uncacheSpan(s)
	}

	// Get a new cached span from the central lists.
	// 该方法会从中心缓存中申请新的 runtime.mspan 存储到线程缓存中，这也是向线程缓存插入内存管理单元的唯一方法
	s = mheap_.central[spc].mcentral.cacheSpan()
	if s == nil {
		throw("out of memory")
	}

	if uintptr(s.allocCount) == s.nelems {
		throw("span has no free space")
	}

	// Indicate that this span is cached and prevent asynchronous
	// sweeping in the next sweep phase.
	s.sweepgen = mheap_.sweepgen + 3

	// Assume all objects from this span will be allocated in the
	// mcache. If it gets uncached, we'll adjust this.
	stats := memstats.heapStats.acquire()
	atomic.Xadduintptr(&stats.smallAllocCount[spc.sizeclass()], uintptr(s.nelems)-uintptr(s.allocCount))

	// Flush tinyAllocs.
	if spc == tinySpanClass {
		atomic.Xadduintptr(&stats.tinyAllocCount, c.tinyAllocs)
		c.tinyAllocs = 0
	}
	memstats.heapStats.release()

	// Update heapLive with the same assumption.
	// While we're here, flush scanAlloc, since we have to call
	// revise anyway.
	usedBytes := uintptr(s.allocCount) * s.elemsize
	gcController.update(int64(s.npages*pageSize)-int64(usedBytes), int64(c.scanAlloc))
	c.scanAlloc = 0
	// 将新申请的span加入到mcache中
	c.alloc[spc] = s
}

// allocLarge allocates a span for a large object.
func (c *mcache) allocLarge(size uintptr, noscan bool) *mspan {
	if size+_PageSize < size {
		throw("out of memory")
	}
	// 根据分配的大小计算需要分配的页数
	npages := size >> _PageShift
	if size&_PageMask != 0 {
		npages++
	}

	// Deduct credit for this span allocation and sweep if
	// necessary. mHeap_Alloc will also sweep npages, so this only
	// pays the debt down to npage pages.
	deductSweepCredit(npages*_PageSize, npages)

	spc := makeSpanClass(0, noscan)
	// 从堆上分配
	s := mheap_.alloc(npages, spc)
	if s == nil {
		throw("out of memory")
	}
	stats := memstats.heapStats.acquire()
	atomic.Xadduintptr(&stats.largeAlloc, npages*pageSize)
	atomic.Xadduintptr(&stats.largeAllocCount, 1)
	memstats.heapStats.release()

	// Update heapLive.
	gcController.update(int64(s.npages*pageSize), 0)

	// Put the large span in the mcentral swept list so that it's
	// visible to the background sweeper.
	mheap_.central[spc].mcentral.fullSwept(mheap_.sweepgen).push(s)
	s.limit = s.base() + size
	heapBitsForAddr(s.base()).initSpan(s)
	return s
}

// 由于 mcache 从非 GC 内存上进行分配，因此出现的任何堆指针都必须进行特殊处理。 所以在释放前，需要调用 mcache.releaseAll 将堆指针进行处理
func (c *mcache) releaseAll() {
	// Take this opportunity to flush scanAlloc.
	scanAlloc := int64(c.scanAlloc)
	c.scanAlloc = 0

	sg := mheap_.sweepgen
	dHeapLive := int64(0)
	for i := range c.alloc {
		s := c.alloc[i]
		if s != &emptymspan {
			// Adjust nsmallalloc in case the span wasn't fully allocated.
			n := uintptr(s.nelems) - uintptr(s.allocCount)
			stats := memstats.heapStats.acquire()
			atomic.Xadduintptr(&stats.smallAllocCount[spanClass(i).sizeclass()], -n)
			memstats.heapStats.release()
			if s.sweepgen != sg+1 {
				// refill conservatively counted unallocated slots in gcController.heapLive.
				// Undo this.
				//
				// If this span was cached before sweep, then
				// gcController.heapLive was totally recomputed since
				// caching this span, so we don't do this for
				// stale spans.
				dHeapLive -= int64(n) * int64(s.elemsize)
			}
			// Release the span to the mcentral.
			mheap_.central[i].mcentral.uncacheSpan(s)
			c.alloc[i] = &emptymspan
		}
	}
	// Clear tinyalloc pool.
	c.tiny = 0
	c.tinyoffset = 0

	// Flush tinyAllocs.
	stats := memstats.heapStats.acquire()
	atomic.Xadduintptr(&stats.tinyAllocCount, c.tinyAllocs)
	c.tinyAllocs = 0
	memstats.heapStats.release()

	// Updated heapScan and heapLive.
	gcController.update(dHeapLive, scanAlloc)
}

// prepareForSweep flushes c if the system has entered a new sweep phase
// since c was populated. This must happen between the sweep phase
// starting and the first allocation from c.
func (c *mcache) prepareForSweep() {
	// Alternatively, instead of making sure we do this on every P
	// between starting the world and allocating on that P, we
	// could leave allocate-black on, allow allocation to continue
	// as usual, use a ragged barrier at the beginning of sweep to
	// ensure all cached spans are swept, and then disable
	// allocate-black. However, with this approach it's difficult
	// to avoid spilling mark bits into the *next* GC cycle.
	sg := mheap_.sweepgen
	if c.flushGen == sg {
		return
	} else if c.flushGen != sg-2 {
		println("bad flushGen", c.flushGen, "in prepareForSweep; sweepgen", sg)
		throw("bad flushGen")
	}
	c.releaseAll()
	stackcache_clear(c)
	atomic.Store(&c.flushGen, mheap_.sweepgen) // Synchronizes with gcStart
}
