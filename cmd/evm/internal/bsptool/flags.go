package bsptool

import (
	"github.com/urfave/cli"
)

var (
	InputBSPFlag = cli.StringFlag{
		Name:  "input.bsp",
		Usage: "`stdin` or file name of where to find the bsp to apply.",
		Value: "bsp.json",
	}
)