package main

import (
	"golang.org/x/net/websocket"
	"io"
)

type jsonRPCRequest struct {
	Id     interface{}
	Method string
	Params []interface{}
}

func RunRPC(ws *websocket.Conn, stream *BroadcastContext) {
	client := ViewerContext{ws, stream}
	client.Open()
	defer client.Close()

	for {
		msg := jsonRPCRequest{Id: nil}
		err := websocket.JSON.Receive(ws, &msg)

		if err != nil {
			if err == io.EOF {
				return
			}

			msg.Error(ws, 0, "parse error")
			continue
		}

		switch msg.Method {
		case "chat_get_history":
			if len(msg.Params) != 0 {
				msg.Error(ws, 0, "invalid argument count")
				continue
			}

			msg.RespondMaybe(ws, client.RequestHistory())

		case "chat_set_name":
			if len(msg.Params) != 1 {
				msg.Error(ws, 0, "invalid argument count")
				continue
			}

			switch name := msg.Params[0].(type) {
			case string:
				msg.RespondMaybe(ws, client.SetName(name))
			default:
				msg.Error(ws, 0, "invalid argument 0 type")
			}

		case "chat_send":
			if len(msg.Params) != 1 {
				msg.Error(ws, 0, "invalid argument count")
				continue
			}

			switch text := msg.Params[0].(type) {
			case string:
				msg.RespondMaybe(ws, client.SendMessage(text))
			default:
				msg.Error(ws, 0, "invalid argument 0 type")
			}

		default:
			msg.Error(ws, 0, "unknown method")
		}
	}
}

func (req *jsonRPCRequest) Respond(ws *websocket.Conn, result interface{}) error {
	return websocket.JSON.Send(ws, map[string]interface{}{
		"json-rpc": "2.0",
		"id":       req.Id,
		"result":   result,
	})
}

func (req *jsonRPCRequest) Error(ws *websocket.Conn, code int, message string) error {
	return websocket.JSON.Send(ws, map[string]interface{}{
		"json-rpc": "2.0",
		"id":       req.Id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
}

func (req *jsonRPCRequest) RespondMaybe(ws *websocket.Conn, err error) error {
	if err == nil {
		return req.Respond(ws, nil)
	} else {
		return req.Error(ws, 0, err.Error())
	}
}

func (ctx *ViewerContext) OnEvent(name string, args []interface{}) error {
	return websocket.JSON.Send(ctx.Socket, map[string]interface{}{
		"json-rpc": "2.0",
		"method":   name,
		"params":   args,
	})
}

func (ctx *ViewerContext) OnMessage(msg ChatMessage) {
	ctx.OnEvent("chat_message", []interface{}{msg.name, msg.text})
}
