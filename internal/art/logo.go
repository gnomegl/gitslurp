package art

import (
	"fmt"
	"runtime/debug"

	"github.com/fatih/color"
)

var version string

func getVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return "unknown"
}

const LogoMain = `   _____   _   _____  _                    
  / ____(_) | /  ___)| |                   
 | |  __| | |_| (___ | |_   _ _ __ _ __  
 | | |_ | | __|\___ \| | | | | '__| '_ \ 
 | |__| | | |_ ____) | | |_| | |  | |_) |
  \_____|_|\__|_____/|_|\__,_|_|  | .__/ 
                                  | |    
                                  |_|  %s`

const LogoText = "v%s by @0xGnomeGL"

func PrintLogo() {
	color.Cyan(fmt.Sprintf(LogoMain, color.HiRedString(fmt.Sprintf(LogoText, version))))
}
