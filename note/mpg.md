# MPG

**G — 表示 Goroutine，它是一个待执行的任务**

**M — 表示操作系统的线程，它由操作系统的调度器调度和管理**

**P — 表示处理器，它可以被看做运行在线程上的本地调度器**

## G
go语言提供的用户态的线程，`runtime.g`

**G 的状态转换** 
![](http://yangxikun.github.io/assets/img/golang-g-status.png)

## M
Go 语言并发模型中的 M 是操作系统线程 `runtime.m`

**M 的状态转换**
![](http://yangxikun.github.io/assets/img/golang-m-create.png)

## P
调度器中的处理器 P 是线程和 Goroutine 的中间层，它能提供线程需要的上下文环境，也会负责调度线程上的等待队列，通过处理器 P 的调度，每一个内核线程都能够执行多个 Goroutine，它能在 Goroutine 进行一些 I/O 操作时及时让出计算资源，提高线程的利用率。`runtime.p`

**P 的状态转换**
![](http://yangxikun.github.io/assets/img/golang-p-status.png)

## m0
m0 是 Go Runtime 所创建的第一个系统线程，一个 Go 进程只有一个 m0，也叫主线程。

从多个方面来看：

数据结构：m0 和其他创建的 m 没有任何区别。

创建过程：m0 是进程在启动时应该汇编直接复制给 m0 的，其他后续的 m 则都是 Go Runtime 内自行创建的。

变量声明：m0 和常规 m 一样，m0 的定义就是 var m0 m，没什么特别之处。


## g0
执行调度任务的叫 g0。

g0 比较特殊，每一个 m 都只有一个 g0（仅此只有一个 g0），且每个 m 都只会绑定一个 g0。在 g0 的赋值上也是通过汇编赋值的，其余后续所创建的都是常规的 g。

从多个方面来看：

数据结构：g0 和其他创建的 g 在数据结构上是一样的，但是存在栈的差别。在 g0 上的栈分配的是系统栈，在 Linux 上栈大小默认固定 8MB，不能扩缩容。 而常规的 g 起始只有 2KB，可扩容。

运行状态：g0 和常规的 g 不一样，没有那么多种运行状态，也不会被调度程序抢占，调度本身就是在 g0 上运行的。

变量声明：g0 和常规 g，g0 的定义就是 var g0 g，没什么特别之处。