package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	ctx := context.Background()

	cmd := "python3"
	args := []string{"/data/data/com.termux/files/home/project/gorky/server/notebooklm_mcp.py"}

	fmt.Printf("Starting %s %v...\n", cmd, args)
	sub := exec.CommandContext(ctx, cmd, args...)
	
	stdin, _ := sub.StdinPipe()
	stdout, _ := sub.StdoutPipe()
	stderr, _ := sub.StderrPipe()

	if err := sub.Start(); err != nil {
		fmt.Printf("Subprocess start failed: %v\n", err)
		return
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Printf("[STDERR] %s\n", scanner.Text())
		}
	}()

	// Spy on Stdout - Write raw bytes to os.Stderr so we can see them
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		tr := io.TeeReader(stdout, os.Stderr)
		io.Copy(pw, tr)
	}()

	fmt.Println("Connecting MCP client...")
	t := transport.NewIO(pr, stdin, nil)
	c := client.NewClient(t)

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	fmt.Println("Sending Initialize request...")
	res, err := c.Initialize(initCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name: "DebugClient",
				Version: "1.0.0",
			},
		},
	})

	if err != nil {
		fmt.Printf("\nFAIL: %v\n", err)
		return
	}

	fmt.Printf("\nSUCCESS: Server version %s\n", res.ServerInfo.Version)
	sub.Process.Kill()
}
