package art

import (
	"fmt"
	"os"

	"git.sr.ht/~gnome/gitslurp/internal/utils"
)

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
	fmt.Fprintf(os.Stderr, "\033[36m%s\033[0m\n", fmt.Sprintf(LogoMain, fmt.Sprintf("\033[91m"+LogoText+"\033[0m", utils.GetVersion())))
	fmt.Fprintln(os.Stderr)
}
