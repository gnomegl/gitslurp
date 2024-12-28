package art

import (
	"fmt"

	"github.com/fatih/color"
)

var version string

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
