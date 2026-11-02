"""Local protocol fixture for browser verification. This is NOT Hermes or an LLM."""
import json
import sys

messages = []
session = "fixture-session"
config = {}


def emit(value):
    print(json.dumps({"jsonrpc": "2.0", **value}), flush=True)


def event(kind, payload):
    emit({"method": "event", "params": {"type": kind, "session_id": session, "payload": payload}})


event("gateway.ready", {})
for line in sys.stdin:
    request = json.loads(line)
    method, params = request.get("method"), request.get("params", {})
    result = {"ok": True}
    if method is None:
        text = "**Fixture 驗證完成。** 已收到核准回應。"
        messages.append({"role": "assistant", "text": text})
        event("tool.complete", {"id": "tool-1", "name": "fixture", "output": "verified"})
        event("message.complete", {"text": text})
        continue
    if method == "session.list":
        result = {"sessions": [{"id": session, "title": "協定測試對話（Fixture）"}] if messages else []}
    elif method in ("session.create", "session.resume", "session.activate"):
        result = {"session_id": session, "messages": messages, "info": {"model": "fixture-model", "provider": "fixture"}}
    elif method == "prompt.submit":
        messages.append({"role": "user", "text": params["text"]})
        event("message.start", {})
        event("message.delta", {"text": "正在驗證串流與工具核准…"})
        event("tool.start", {"id": "tool-1", "name": "fixture", "description": "不執行任何系統命令"})
        emit({"id": "fixture-approval", "method": "approval", "params": {"session_id": session, "command": "fixture: verify protocol", "description": "僅驗證協定，沒有實際執行命令。"}})
    elif method == "session.interrupt":
        event("request.cancel", {"id": "fixture-approval"})
        event("message.complete", {"text": "Fixture 已中止。"})
    elif method == "model.options":
        result = {"models": ["fixture-model"]}
    elif method == "config.set":
        config[params["key"]] = params["value"]
    elif method == "config.get":
        result = {"value": config.get(params["key"], "")}
    emit({"id": request["id"], "result": result})
