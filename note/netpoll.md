# netpoll

## 网络轮询器的初始化
因为文件 I/O、网络 I/O 以及计时器都依赖网络轮询器，所以 Go 语言会通过以下两条不同路径初始化网络轮询器
- internal/poll.pollDesc.init — 通过 net.netFD.init 和 os.newFile 初始化网络 I/O 和文件 I/O 的轮询信息时
- runtime.doaddtimer — 向处理器中增加新的计时器时
- 网络轮询器的初始化会使用 runtime.poll_runtime_pollServerInit 和 runtime.netpollGenericInit 两个函数

```go
func poll_runtime_pollServerInit() {
	netpollGenericInit()
}

func netpollGenericInit() {
	if atomic.Load(&netpollInited) == 0 {
		lock(&netpollInitLock)
		if netpollInited == 0 {
			netpollinit()
			atomic.Store(&netpollInited, 1)
		}
		unlock(&netpollInitLock)
	}
}
```

## 轮询事件
- Goroutine 让出线程并等待读写事件；

当我们在文件描述符上执行读写操作时，如果文件描述符不可读或者不可写，当前 Goroutine 会执行 runtime.poll_runtime_pollWait 检查 runtime.pollDesc 的状态并调用 runtime.netpollblock 等待文件描述符的可读或者可写：

runtime.netpollblock 是 Goroutine 等待 I/O 事件的关键函数，它会使用运行时提供的 runtime.gopark 让出当前线程，将 Goroutine 转换到休眠状态并等待运行时的唤醒。

- 多路复用等待读写事件的发生并返回；

Go 语言的运行时会在调度或者系统监控中调用 runtime.netpoll 轮询网络，该函数的执行过程可以分成以下几个部分：

根据传入的 delay 计算 epoll 系统调用需要等待的时间；

调用 epollwait 等待可读或者可写事件的发生；

在循环中依次处理 epollevent 事件；