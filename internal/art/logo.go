package art

import (
	"fmt"
	"os"

	"github.com/common-nighthawk/go-figure"
	"github.com/gnomegl/gitslurp/internal/utils"
)

func PrintLogo() {
	myFigure := figure.NewFigure("gitslurp", "chunky", false)
	fmt.Fprintf(os.Stderr, "\033[36m%s\033[0m", myFigure.String())
	fmt.Fprintf(os.Stderr, "              \033[91mv%s by gnomegl\033[0m\n\n", utils.GetVersion())
}
