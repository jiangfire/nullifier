//go:build !js || !wasm

// Command ballot-wasm 是浏览器端的选票 prover，只在 GOOS=js GOARCH=wasm 下构建。
//
// 这个文件是非 js/wasm 平台的占位实现：让 `go build ./...`、`go vet ./...`
// 与 `go test ./...` 在开发机与 CI 上不必为浏览器构建单独开一套流程。
// 真正的构建见 `make web-wasm`。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "ballot-wasm 只能在 GOOS=js GOARCH=wasm 下构建（见 make web-wasm）")
	os.Exit(2)
}
