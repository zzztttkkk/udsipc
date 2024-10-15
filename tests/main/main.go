package main

import "github.com/zzztttkkk/udsipc"

func main() {
	ipc := udsipc.NewUnixIpc(udsipc.UdsIpcOpts{
		FilePath:        "./ipc.sock",
		Secret:          "123456",
		PingStepInMills: 1000,
	})

	ipc.Run()
}
