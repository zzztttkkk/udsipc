package main

import (
	"fmt"

	"github.com/zzztttkkk/udsipc"
)

func main() {
	fmt.Println(udsipc.IsSupported())

	ipc := udsipc.NewUnixIpc(udsipc.UdsIpcOpts{
		FilePath:        "./ipc.sock",
		Secret:          "123456",
		PingStepInMills: 1000,
	})

	ipc.Run()
}
