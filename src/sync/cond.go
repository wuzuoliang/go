// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sync

import (
	"sync/atomic"
	"unsafe"
)

// Cond implements a condition variable, a rendezvous point
// for goroutines waiting for or announcing the occurrence
// of an event.
//
// Each Cond has an associated Locker L (often a *Mutex or *RWMutex),
// which must be held when changing the condition and
// when calling the Wait method.
//
// A Cond must not be copied after first use.
type Cond struct {
	noCopy noCopy

	// L is held while observing or changing the condition
	L Locker
	// 通知列表,调用Wait()方法的goroutine会被放入list中,每次唤醒,从这里取出
	notify  notifyList
	checker copyChecker
}

// NewCond returns a new Cond with Locker l.
func NewCond(l Locker) *Cond {
	return &Cond{L: l}
}

// Wait atomically unlocks c.L and suspends execution
// of the calling goroutine. After later resuming execution,
// Wait locks c.L before returning. Unlike in other systems,
// Wait cannot return unless awoken by Broadcast or Signal.
//
// Because c.L is not locked when Wait first resumes, the caller
// typically cannot assume that the condition is true when
// Wait returns. Instead, the caller should Wait in a loop:
//
//    c.L.Lock()
//    for !condition() {
//        c.Wait()
//    }
//    ... make use of condition ...
//    c.L.Unlock()
//
/**
// https://geektutu.com/post/hpg-sync-cond.html
// https://blog.csdn.net/qq_39787367/article/details/108752611
调用 Wait 会自动释放锁 c.L，并挂起调用者所在的 goroutine，因此当前协程会阻塞在 Wait 方法调用的地方。
如果其他协程调用了 Signal 或 Broadcast 唤醒了该协程，那么 Wait 方法在结束阻塞时，会重新给 c.L 加锁，并且继续执行 Wait 后面的代码。
对条件的检查，使用了 for !condition() 而非 if，是因为当前协程被唤醒时，
条件不一定符合要求，需要再次 Wait 等待下次被唤醒。为了保险起见，使用 for 能够确保条件符合要求后，再执行后续的代码。

为什么先要锁定条件变量基于的互斥锁，才能调用它的Wait方法?
因为条件变量的Wait方法在阻塞当前的 goroutine 之前，会解锁它基于的互斥锁，所以在调用该Wait方法之前，我们必须先锁定那个互斥锁，否则在调用这个Wait方法时，就会引发一个不可恢复的 panic。
为什么条件变量的Wait方法要这么做呢？你可以想象一下，如果Wait方法在互斥锁已经锁定的情况下，阻塞了当前的 goroutine，那么又由谁来解锁呢？别的 goroutine 吗？
先不说这违背了互斥锁的重要使用原则，即：成对的锁定和解锁，就算别的 goroutine 可以来解锁，那万一解锁重复了怎么办？由此引发的 panic 可是无法恢复的。
如果当前的 goroutine 无法解锁，别的 goroutine 也都不来解锁，那么又由谁来进入临界区，并改变共享资源的状态呢？只要共享资源的状态不变，即使当前的 goroutine 因收到通知而被唤醒，也依然会再次执行这个Wait<方法，并再次被阻塞。
所以说，如果条件变量的Wait方法不先解锁互斥锁的话，那么就只会造成两种后果：不是当前的程序因 panic 而崩溃，就是相关的 goroutine 全面阻塞。

为什么要用for语句来包裹调用其Wait方法的表达式，用if不可以吗?
显然，if语句只会对共享资源的状态检查一次，而for语句却可以做多次检查，直到这个状态改变为止。那为什么要做多次检查呢？
这主要是为了保险起见。如果一个 goroutine 因收到通知而被唤醒，但却发现共享资源的状态，依然不符合它的要求，那么就应该再次调用条件变量的Wait方法，并继续等待下次通知的到来。
这种情况是很有可能发生的
*/
func (c *Cond) Wait() {
	// 检查c是否是被复制的，如果是就panic
	c.checker.check()
	// 将当前goroutine加入等待队列
	t := runtime_notifyListAdd(&c.notify)
	// 解锁
	c.L.Unlock()
	// 等待队列中的所有的goroutine执行等待唤醒操作
	runtime_notifyListWait(&c.notify, t)
	c.L.Lock()
}

// Signal wakes one goroutine waiting on c, if there is any.
//
// It is allowed but not required for the caller to hold c.L
// during the call.
func (c *Cond) Signal() {
	// 检查c是否是被复制的，如果是就panic
	c.checker.check()
	// 通知等待列表中的一个
	runtime_notifyListNotifyOne(&c.notify)
}

// Broadcast wakes all goroutines waiting on c.
//
// It is allowed but not required for the caller to hold c.L
// during the call.
func (c *Cond) Broadcast() {
	// 检查c是否是被复制的，如果是就panic
	c.checker.check()
	// 唤醒等待队列中所有的goroutine
	runtime_notifyListNotifyAll(&c.notify)
}

// copyChecker holds back pointer to itself to detect object copying.
type copyChecker uintptr

func (c *copyChecker) check() {
	if uintptr(*c) != uintptr(unsafe.Pointer(c)) &&
		!atomic.CompareAndSwapUintptr((*uintptr)(c), 0, uintptr(unsafe.Pointer(c))) &&
		uintptr(*c) != uintptr(unsafe.Pointer(c)) {
		panic("sync.Cond is copied")
	}
}

// noCopy may be embedded into structs which must not be copied
// after the first use.
//
// See https://golang.org/issues/8005#issuecomment-190753527
// for details.
type noCopy struct{}

// Lock is a no-op used by -copylocks checker from `go vet`.
func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
