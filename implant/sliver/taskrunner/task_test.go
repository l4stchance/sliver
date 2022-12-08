package taskrunner

import (
	"io/ioutil"
	"testing"
	"time"
)

// 如果要注入的进程并不会驻留一段时间，那么很有可能Shellcode并不会执行，程序会直接结束
func TestSpawnDLL(t *testing.T) {
	b, err := ioutil.ReadFile("C:\\reflective_dll.x64.dll")
	if err != nil {
		return
	}
	// 正常执行，进程不退出
	SpawnDll("C:\\Windows\\system32\\Notepad.exe", []string{}, 19252, b, 1100, "", false)
	// 未正常执行，进程直接退出了
	SpawnDll("C:\\Windows\\system32\\ping.exe", []string{}, 19252, b, 1100, "", false)

	for {
		time.Sleep(20 * time.Second)
	}
}
