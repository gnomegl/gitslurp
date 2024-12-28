package art

import (
	"fmt"

	"github.com/fatih/color"
)

// based: injected at build time
var version = "dev"

const LogoMain = `   _____   _   _____  _                    
  / ____(_) | /  ___)| |                   
 | |  __| | |_| (___ | |_   _ _ __ _ __  
 | | |_ | | __|\___ \| | | | | '__| '_ \ 
 | |__| | | |_ ____) | | |_| | |  | |_) |
  \_____|_|\__|_____/|_|\__,_|_|  | .__/ 
                                  | |    
                                  |_|    %s`

const LogoText = "v%s by gnomegl"

// PrintLogo prints the gitslurp logo in color
func PrintLogo() {
	color.Cyan(fmt.Sprintf(LogoMain, color.HiYellowString(fmt.Sprintf(LogoText, version))))
}
