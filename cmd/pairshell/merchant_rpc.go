package main

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"time"

	"golang.org/x/crypto/ssh"
)

const rpcHeaderLimit = 16 << 10

func merchantRPC(client *ssh.Client, request *http.Request, bodyLimit int64, timeout time.Duration) (int, []byte, error) {
	// SSH channels do not expose deadlines. Close the authenticated peer
	// connection if it stalls even during the channel-open handshake.
	timer := time.AfterFunc(timeout, func() { client.Close() })
	defer timer.Stop()
	ch, requests, err := client.OpenChannel("pairshell-rpc", nil)
	if err != nil {
		return 0, nil, err
	}
	defer ch.Close()
	go ssh.DiscardRequests(requests)
	if err := request.Write(ch); err != nil {
		return 0, nil, err
	}
	return readRPCResponse(ch, request, bodyLimit)
}

func readRPCResponse(src io.Reader, request *http.Request, bodyLimit int64) (int, []byte, error) {
	wire := &io.LimitedReader{R: src, N: rpcHeaderLimit}
	response, err := http.ReadResponse(bufio.NewReader(wire), request)
	if err != nil {
		return 0, nil, err
	}
	if response.StatusCode < 200 || response.StatusCode > 599 || response.ContentLength > bodyLimit {
		return 0, nil, errors.New("invalid RPC response")
	}
	// Bound encoded framing as well as decoded body size. The caller closes
	// the channel directly, so an oversized body is never drained.
	wire.N += bodyLimit + rpcHeaderLimit + 1
	body, err := io.ReadAll(io.LimitReader(response.Body, bodyLimit+1))
	if err != nil {
		return 0, nil, err
	}
	if int64(len(body)) > bodyLimit || wire.N == 0 {
		return 0, nil, errors.New("RPC response exceeds size limit")
	}
	return response.StatusCode, body, nil
}
