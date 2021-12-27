# schedule

## 调度器版本演进
```
单线程调度器 0.x
- 程序中只能存在一个活跃线程，由 G-M 模型组成

多线程调度器 1.0
- 允许运行多线程的程序
- 全局锁导致竞争严重

任务窃取调度器 1.1
- 引入了处理器 P，构成了目前的 G-M-P 模型
- 在处理器 P 的基础上实现了基于工作窃取的调度器
- 在某些情况下，Goroutine 不会让出线程，进而造成饥饿问题
- 时间过长的垃圾回收（Stop-the-world，STW）会导致程序长时间无法工作

抢占式调度器 1.2>
- 基于协作的抢占式调度器 1.2~1.3
    - 通过编译器在函数调用时插入抢占检查指令，在函数调用时检查当前 Goroutine 是否发起了抢占请求，实现基于协作的抢占式调度
    - Goroutine 可能会因为垃圾回收和循环长时间占用资源导致程序暂停
- 基于信号的抢占式调度器 1.14>
    - 实现基于信号的真抢占式调度
    - 垃圾回收在扫描栈时会触发抢占调度
    - 抢占的时间点不够多，还不能覆盖全部的边缘情况
```

## 抢占式调度
- 基于协作的抢占式调度
```
主动用户让权：通过 runtime.Gosched 调用主动让出执行机会；

主动调度弃权：编译器会在调用函数前插入 runtime.morestack；

Go 语言运行时会在垃圾回收暂停程序、系统监控发现 Goroutine 运行超过 10ms 时发出抢占请求 StackPreempt；
    
当发生函数调用时，可能会执行编译器插入的 runtime.morestack，它调用的 runtime.newstack 会检查 Goroutine 的 stackguard0 字段是否为 StackPreempt；
    
如果 stackguard0 是 StackPreempt，就会触发抢占让出当前线程；
```
- 基于信号的抢占式调度
```
被动监控抢占：
程序启动时，在 runtime.sighandler 中注册 SIGURG 信号的处理函数 runtime.doSigPreempt；

在触发垃圾回收的栈扫描时会调用 runtime.suspendG 挂起 Goroutine，该函数会执行下面的逻辑：

将 _Grunning 状态的 Goroutine 标记成可以被抢占，即将 preemptStop 设置成 true；

调用 runtime.preemptM 触发抢占；

runtime.preemptM 会调用 runtime.signalM 向线程发送信号 SIGURG；

操作系统会中断正在运行的线程并执行预先注册的信号处理函数 runtime.doSigPreempt；

runtime.doSigPreempt 函数会处理抢占信号，获取当前的 SP 和 PC 寄存器并调用 runtime.sigctxt.pushCall；

runtime.sigctxt.pushCall 会修改寄存器并在程序回到用户态时执行 runtime.asyncPreempt；

汇编指令 runtime.asyncPreempt 会调用运行时函数 runtime.asyncPreempt2；

runtime.asyncPreempt2 会调用 runtime.preemptPark；

runtime.preemptPark 会修改当前 Goroutine 的状态到 _Gpreempted 并调用 runtime.schedule 让当前函数陷入休眠并让出线程，调度器会选择其它的 Goroutine 继续执行；

被动 GC 抢占：当需要进行垃圾回收时，为了保证不具备主动抢占处理的函数执行时间过长，导致 导致垃圾回收迟迟不得执行而导致的高延迟，而强制停止 G 并转为执行垃圾回收。
```

## flow
![goroutine 创建](http://yangxikun.github.io/assets/img/goroutine-create.png)

调度器循环纵览 ![](https://golang.design/under-the-hood/assets/schedule.png)