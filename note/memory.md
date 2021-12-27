# memory

![Go 内存管理结构总览](https://golang.design/under-the-hood/assets/mem-struct.png)

heap 最中间的灰色区域 arena 覆盖了 Go 程序的整个虚拟内存， 每个 arena 包括一段 bitmap 和一段指向连续 span 的指针； 每个 span 由一串连续的页组成；每个 arena 的起始位置通过 arenaHint 进行记录。

分配的顺序从右向左，代价也就越来越大。 小对象和微对象优先从白色区域 per-P 的 mcache 分配 span，这个过程不需要加锁（白色）； 若失败则会从 mheap 持有的 mcentral 加锁获得新的 span，这个过程需要加锁，但只是局部（灰色）； 若仍失败则会从右侧的 free 或 scav 进行分配，这个过程需要对整个 heap 进行加锁，代价最大（黑色）。

![1](https://img.int64.ink/855588850909175216.png)

Golang中堆是以一块一块的内存块组成的，在runtime中的表示为heapArena，每一块大小为64MB，由很多页组成，一页占据8KB的空间(64bit下)。
mheap是对内存块的管理对象，它按照page为粒度进行管理。
mcentral是对mheap中的page进一步管理，因为它是全局的，所以访问需要加锁。每一种mspan规格对应一个mcentral来进行管理。每个mcentral有两个mspan列表，一个是未使用的，一个是已使用的。
在每个P内部维护了一个P自身的缓存mcache，mcache也维护了67种不同规格的mspan列表，每个列表存储在一个数组中。一种规格对应数组的两个元素，一个存储指针对象，一个是存储非指针对象。mcache本身由P独占，所以无须加锁申请。
在程序中申请内存时(如make，或者new等)，所在g对应的P会先从自身的mcache申请，如果没有足够的空间，则向mcentral申请，如果还是没有，则继续向mheap申请。
因为mspan的规格只有从1Byte到32KB，所以对于超过32KB的对象，会直接向mheap申请。
Golang的内存分配采用多级缓存，减少锁粒度，以达到更高效的内存分配效率。回收对象时也并非直接回收内存，而是放回预先分配的内存块中，只有空闲的内存太多时才会归还给操作系统。


## start
schedinit -> mallocinit