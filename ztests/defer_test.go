/*
这是一个演示golang defer特性的示例. 仔细看输出数据,才能准确理解defer特点.
基本总结就是:
+	在defer语句被执行时,首先要准确生成一个func的definition,并将这个func注册到隐含的finally块.
	此时对func必须完成parse, parameter确定下来(函数的receiver是一个隐含的parameter),
	closure的所有stub也要生成.
 */
package ztests

import (
	"fmt"
	"os"
	"testing"
)

func fa(x, y int) func(int, int) {
	fmt.Fprintf(os.Stdout, "fa \t apply \t %d %d \n", x, y)
	return func(x1, y1 int) { // fb
		fmt.Fprintf(os.Stdout, "fb \t apply \t %d %d \n", x1, y1)
	}
}

func FX(a int) (b int) {
	defer fa(a, b)
	defer func() { panic("p1") }()
	defer fa(a, b)
	defer func() { panic("p2") }()
	defer fa(a, b)(a, b)
	// fc
	defer func() { fmt.Fprintf(os.Stdout, "fc \t apply \t %d %d \n", a, b) }()
	a += 1
	defer fa(a, b)
	// fd
	defer func(a, b int) { fmt.Fprintf(os.Stdout, "fd \t apply \t %d %d \n", a, b) }(a, b)
	b = a * 2
	// fe
	defer fa(a,
		func() int { fmt.Fprintf(os.Stdout, "fe \t apply \t %d %d \n", a, b); return b }())
	// ff
	defer func() { b += 1; fmt.Fprintf(os.Stdout, "ff \t apply \t %d %d \n", a, b) }()

	fmt.Fprintf(os.Stdout, "FX \t apply \t %d %d \n", a, b)
	return
}

func TestDefer(t *testing.T) {
	FX(1)
}

/*
=== RUN   TestDefer
fa       apply   1 0
fe       apply   2 4
FX       apply   2 4
ff       apply   2 5
fa       apply   2 4
fd       apply   2 0
fa       apply   2 0
fc       apply   2 5
fb       apply   1 0
fa       apply   1 0
--- PASS: TestDefer (0.00s)

 */
