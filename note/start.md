# go程序启动流程

vim hello.go
```go
package main
import "fmt"
func main(){
    fmt.Println("hello go")
}
```
go build hello.go

readelf -h ./hello

or

gdb hello -> info files

```
ELF Header:
  Magic:   7f 45 4c 46 02 01 01 00 00 00 00 00 00 00 00 00
  Class:                             ELF64
  Data:                              2's complement, little endian
  Version:                           1 (current)
  OS/ABI:                            UNIX - System V
  ABI Version:                       0
  Type:                              EXEC (Executable file)
  Machine:                           Advanced Micro Devices X86-64
  Version:                           0x1
  Entry point address:               0x464840
  Start of program headers:          64 (bytes into file)
  Start of section headers:          456 (bytes into file)
  Flags:                             0x0
  Size of this header:               64 (bytes)
  Size of program headers:           56 (bytes)
  Number of program headers:         7
  Size of section headers:           64 (bytes)
  Number of section headers:         25
  Section header string table index: 3
```
dlv exec ./hello
```
Type 'help' for list of commands.
(dlv) b *0x464840
Breakpoint 1 set at 0x464840 for _rt0_amd64_linux() .usr/lib/golang/src/runtime/rt0_linux_amd64.s:8
(dlv) l
> _rt0_amd64_linux() /usr/lib/golang/src/runtime/rt0_linux_amd64.s:8 (PC: 0x464780)
Warning: debugging optimized function
     3:	// license that can be found in the LICENSE file.
     4:
     5:	#include "textflag.h"
     6:
     7:	TEXT _rt0_amd64_linux(SB),NOSPLIT,$-8
=>   8:		JMP	_rt0_amd64(SB)
     9:
    10:	TEXT _rt0_amd64_linux_lib(SB),NOSPLIT,$0
    11:		JMP	_rt0_amd64_lib(SB)
(dlv) si
> _rt0_amd64() /usr/lib/golang/src/runtime/asm_amd64.s:15 (PC: 0x4612a0)
Warning: debugging optimized function
    10:	// _rt0_amd64 is common startup code for most amd64 systems when using
    11:	// internal linking. This is the entry point for the program from the
    12:	// kernel for an ordinary -buildmode=exe program. The stack holds the
    13:	// number of arguments and the C-style argv.
    14:	TEXT _rt0_amd64(SB),NOSPLIT,$-8
    15:		MOVQ	0(SP), DI	// argc
    16:		LEAQ	8(SP), SI	// argv
=>  17:		JMP	runtime·rt0_go(SB)
    18:
    19:	// main is common startup code for most amd64 systems when using
    20:	// external linking. The C startup code will call the symbol "main"
(dlv) si
> runtime.rt0_go() /usr/lib/golang/src/runtime/asm_amd64.s:89 (PC: 0x4612c0)
Warning: debugging optimized function
    84:	DATA _rt0_amd64_lib_argv<>(SB)/8, $0
    85:	GLOBL _rt0_amd64_lib_argv<>(SB),NOPTR, $8
    86:
    87:	TEXT runtime·rt0_go(SB),NOSPLIT,$0
    88:		// copy arguments forward on an even stack
    89:		MOVQ	DI, AX		// argc
    90:		MOVQ	SI, BX		// argv
    91:		SUBQ	$(4*8+7), SP		// 2args 2auto
    92:		ANDQ	$~15, SP
    93:		MOVQ	AX, 16(SP)
    94:		MOVQ	BX, 24(SP)
....
   212:		CALL	runtime·args(SB)
=> 213:		CALL	runtime·osinit(SB)
   214:		CALL	runtime·schedinit(SB)
   215:
   216:		// create a new goroutine to start program
   217:		MOVQ	$runtime·mainPC(SB), AX		// entry
   218:		PUSHQ	AX
   219:		PUSHQ	$0			// arg size
=> 220:		CALL	runtime·newproc(SB)
   221:		POPQ	AX
   222:		POPQ	AX
   223:
   224:		// start this M
=> 225:		CALL	runtime·mstart(SB)
```

## 汇编启动大概流程
`runtime._rt0_amd64_linux`->`runtime._rt0_amd64`->`runtime.rt0_go`->`runtime·args`->`runtime·osinit`->`runtime·schedinit`
->`runtime.mainPC`->`runtime·newproc`->`runtime·mstart`

## go启动流程图 
![](http://yangxikun.github.io/assets/img/201911220101.png)
