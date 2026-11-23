package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"pairshell/internal/pairing"
)

func verifyPeerFromInput(ctx context.Context, input *bufio.Reader, output io.Writer, expected string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Fprintf(output, "本机会话校验码：%s\n通过你已信任的电话或聊天，请对方报出其 PairShell 界面上的完整校验码。输入对方读出的校验码以继续，Ctrl+C 取消：\n", expected)
	answer := make(chan string, 1)
	go func() {
		line, err := input.ReadSlice('\n')
		if err != nil {
			answer <- ""
			return
		}
		answer <- string(line)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case line := <-answer:
		if err := ctx.Err(); err != nil {
			return err
		}
		if !pairing.MatchVerificationCode(expected, line) {
			return errors.New("会话校验码不一致或未完成核对，已拒绝连接")
		}
		return nil
	}
}
