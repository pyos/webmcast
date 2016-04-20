package main

import (
	"encoding/json"
	"errors"
	"github.com/powerman/rpc-codec/jsonrpc2"
	"golang.org/x/net/websocket"
	"net/rpc"
)

type RPCSingleStringArg struct {
	First string
}

func (x *RPCSingleStringArg) UnmarshalJSON(buf []byte) error {
	fields := []interface{}{&x.First}
	expect := len(fields)
	if err := json.Unmarshal(buf, &fields); err != nil {
		return err
	}
	if len(fields) != expect {
		return errors.New("invalid number of arguments")
	}
	return nil
}

func RPCPushEvent(ws *websocket.Conn, name string, args []interface{}) error {
	return websocket.JSON.Send(ws, map[string]interface{}{
		"jsonrpc": "2.0", "method": name, "params": args,
	})
}

func (stream *BroadcastContext) RunRPC(ws *websocket.Conn, user *UserShortData) {
	chatter := stream.Chat.Connect(ws, user)
	defer stream.Chat.Disconnect(chatter)

	server := rpc.NewServer()
	server.RegisterName("Chat", chatter)
	server.ServeCodec(jsonrpc2.NewServerCodec(ws, server))
}
