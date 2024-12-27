package art

import "github.com/fatih/color"

const Logo = `   _____   _   _____  _                    
  / ____(_) | / ____)| |                   
 | |  __| | |_|(___  | |_   _ _ __ _ __  
 | | |_ | | __|\___ \| | | | | '__| '_ \ 
 | |__| | | |_ ____) | | |_| | |  | |_) |
  \_____|_|\__|_____/|_|\__,_|_|  | .__/ 
                                  | |    
                                  |_|    `

// PrintLogo prints the gitslurp logo in color
func PrintLogo() {
	color.Cyan(Logo)
}
