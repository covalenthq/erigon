package t8ntool

import (
	"fmt"
	"os"
	"runtime"
)

const (
	EvmServerVersionMajor = 1
	EvmServerVersionMinor = 1
	EvmServerVersionPatch = 0
	clientIdentifier      = "evm-server" // Client identifier to advertise over the network
)

// EvmServerVersion holds the textual version string.
var EvmServerVersion = func() string {
	return fmt.Sprintf("%d.%d.%d", EvmServerVersionMajor, EvmServerVersionMinor, EvmServerVersionPatch)
}()

// Version Provides version info on bsp agent binary
func Version() {
	fmt.Println(clientIdentifier)
	fmt.Println("evm-server Version:", EvmServerVersion)
	fmt.Println("Architecture:", runtime.GOARCH)
	fmt.Println("Go Version:", runtime.Version())
	fmt.Println("Operating System:", runtime.GOOS)
	fmt.Printf("GOPATH=%s\n", os.Getenv("GOPATH"))
	fmt.Printf("GOROOT=%s\n", runtime.GOROOT())
}
