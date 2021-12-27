# runtime

--- 

## MPG模型
### G
Goroutine 是 Go 语言调度器中待执行的任务，`runtime.g`
![Goroutine 的常见状态迁移](https://golang.design/under-the-hood/assets/g-status.png)

| 状态        | 描述    |状态|
| --------   | :----- |:---|
|_Gidle|刚刚被分配并且还没有被初始化||
|_Grunnable	|没有执行代码，没有栈的所有权，存储在运行队列中|可运行|
|_Grunning|	可以执行代码，拥有栈的所有权，被赋予了内核线程 M 和处理器 P|运行中|
|_Gsyscall|	正在执行系统调用，拥有栈的所有权，没有执行用户代码，被赋予了内核线程 M 但是不在运行队列上|等待中|
|_Gwaiting|	由于运行时而被阻塞，没有执行用户代码并且不在运行队列上，但是可能存在于 Channel 的等待队列上|等待中|
|_Gdead	|没有被使用，没有执行代码，可能有分配的栈||
|_Gcopystack|	栈正在被拷贝，没有执行代码，不在运行队列上||
|_Gpreempted|	由于抢占而被阻塞，没有执行用户代码并且不在运行队列上，等待唤醒|等待中|
|_Gscan	|GC 正在扫描栈空间，没有执行代码，可以与其他状态同时存在||

### M
Go 语言并发模型中的 M 是操作系统线程,`runtime.m`

![调度 Goroutine 和运行 Goroutine](https://img.draveness.me/2020-02-05-15808864354644-g0-and-g.png)

### P
调度器中的处理器 P 是线程和 Goroutine 的中间层，它能提供线程需要的上下文环境，也会负责调度线程上的等待队列，通过处理器 P 的调度，每一个内核线程都能够执行多个 Goroutine，它能在 Goroutine 进行一些 I/O 操作时及时让出计算资源，提高线程的利用率。`runtime.p`

| 状态        | 描述    |
| ---|:---|
|_Pidle	|处理器没有运行用户代码或者调度器，被空闲队列或者改变其状态的结构持有，运行队列为空|
|_Prunning|	被线程 M 持有，并且正在执行用户代码或者调度器|
|_Psyscall|	没有执行用户代码，当前线程陷入系统调用|
|_Pgcstop	|被线程 M 持有，当前处理器由于垃圾回收被停止|
|_Pdead|	当前处理器已经不被使用|

![P的状态转换](https://golang.design/under-the-hood/assets/p-status.png)
--- 

### 定时器 timers
#### 演进
- 全局四叉堆：全局共用互斥锁加锁性能牺牲很大
- 分片四叉堆：虽然能够降低锁的粒度，提高计时器的性能，但是造成的处理器和线程之间频繁的上下文切换却成为了影响计时器性
- 网络轮询器：所有的计时器都以最小四叉堆的形式存储在处理器 runtime.p 中，计时器都交由处理器的网络轮询器和调度器触发，这种方式能够充分利用本地性、减少上下文的切换开销

#### 计时器状态机
| 状态        | 描述   |  其他信息 |
| --------   | :-----  | :----  |
|timerNoStatus	|还没有设置状态|计时器不在堆上|
|timerWaiting	|等待触发|计时器在处理器的堆上|
|timerRunning	|运行计时器函数|停留的时间都比较短|
|timerDeleted	|被删除|计时器在处理器的堆上|
|timerRemoving	|正在被删除|停留的时间都比较短|
|timerRemoved	|已经被停止并从堆中删除|计时器不在堆上|
|timerModifying	|正在被修改|停留的时间都比较短|
|timerModifiedEarlier|	被修改到了更早的时间|计时器虽然在堆上，但是可能位于错误的位置上，需要重新排序|
|timerModifiedLater	|被修改到了更晚的时间|计时器虽然在堆上，但是可能位于错误的位置上，需要重新排序|
|timerMoving	|已经被修改正在被移动|停留的时间都比较短|

![计时器状态机](https://golang.design/under-the-hood/assets/timers.png)

#### 触发时机
- 调度器调度时会检查处理器中的计时器是否准备就绪 `runtime.checkTimers`
- 系统监控会检查是否有未执行的到期计时器 `runtime.sysmon`

---
### 调度器

#### 演进
- 单线程调度器
- 多线程调度器
- 任务窃取调度器
- 抢占式调度器
  - 基于协作的抢占式
  - 基于信号的抢占式

#### 调度器结构 `schedt`
- 管理了能够将 G 和 M 进行绑定的 M 队列
- 管理了空闲的 P 链表（队列）
- 管理了 G 的全局队列
- 管理了可被复用的 G 的全局缓存
- 管理了 defer 池

#### 调度器启动
- func schedinit()
![初始化](https://golang.design/under-the-hood/assets/sched-init.png)
```
TEXT runtime·rt0_go(SB),NOSPLIT,$0
	(...)
	CALL	runtime·schedinit(SB) // M, P 初始化
	MOVQ	$runtime·mainPC(SB), AX
	PUSHQ	AX
	PUSHQ	$0
	CALL	runtime·newproc(SB) // G 初始化
	POPQ	AX
	POPQ	AX
	(...)
	// 启动 M
	CALL	runtime·mstart(SB) // 开始执行
	RET

DATA	runtime·mainPC+0(SB)/8,$runtime·main(SB)
GLOBL	runtime·mainPC(SB),RODATA,$8
```

### 执行调度
- runtime.mstart() -> ... -> func schedule()

  mstart 除了在程序引导阶段会被运行之外，也可能在每个 m 被创建时运行；

  mstart 进入 mstart1 之后，会初始化自身用于信号处理的 g，在 mstartfn 指定时将其执行；

  调度循环 schedule 无法返回，因此最后一个 mexit 目前还不会被执行，因此当下所有的 Go 程序创建的线程都无法被释放

![调度时间点](https://img.draveness.me/2020-02-05-15808864354679-schedule-points.png)
![调度器调度循环纵览](https://golang.design/under-the-hood/assets/schedule.png)

### 协作与抢占

在调度器的初始化过程中，首先通过 mcommoninit 对 M 的信号 G 进行初始化。 而后通过 procresize 创建与 CPU 核心数 (或与用户指定的 GOMAXPROCS) 相同的 P。 最后通过 newproc 创建包含可以运行要执行函数的执行栈、运行现场的 G，并将创建的 G 放入刚创建好的 P 的本地可运行队列（第一个入队的 G，也就是主 Goroutine 要执行的函数体）， 完成 G 的创建。
### 网络轮询器
- func ...netpoll...()

### 系统监控
- func sysmon()
    - 死锁检查
    - 定时器检查
    - 轮询网络
    - 抢占处理器
    - 垃圾回收