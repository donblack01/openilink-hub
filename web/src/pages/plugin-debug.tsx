import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Card } from "../components/ui/card";
import { api } from "../lib/api";
import { Play, ChevronDown } from "lucide-react";

const defaultScript = `// ==WebhookPlugin==
// @name         调试插件
// @version      1.0.0
// @match        text
// @connect      *
// @grant        reply
// ==/WebhookPlugin==

function onRequest(ctx) {
  ctx.req.body = JSON.stringify({
    text: ctx.msg.sender + ": " + ctx.msg.content
  });
}

function onResponse(ctx) {
  if (ctx.res.status === 200) {
    reply("收到响应: " + ctx.res.status);
  }
}`;

export function PluginDebugPage() {
  const [searchParams] = useSearchParams();
  const [script, setScript] = useState(defaultScript);
  const [webhookUrl, setWebhookUrl] = useState("https://httpbin.org/post");
  const [sender, setSender] = useState("test_user@debug");
  const [content, setContent] = useState("Hello from debug");
  const [msgType, setMsgType] = useState("text");
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<any>(null);
  const [plugins, setPlugins] = useState<any[]>([]);
  const [showPicker, setShowPicker] = useState(false);
  const [myPlugins, setMyPlugins] = useState<any[]>([]);

  // Load plugin list for picker
  useEffect(() => {
    Promise.all([
      api.listPlugins().catch(() => []),
      api.myPlugins().catch(() => []),
    ]).then(([pub, mine]) => {
      setPlugins(pub || []);
      setMyPlugins(mine || []);
    });
  }, []);

  // Load plugin from URL param ?plugin=id
  useEffect(() => {
    const pluginId = searchParams.get("plugin");
    if (pluginId) {
      api.getPlugin(pluginId).then((p) => {
        if (p.script) setScript(p.script);
      }).catch(() => {});
    }
  }, [searchParams]);

  async function handleRun() {
    setRunning(true);
    setResult(null);
    const mockMessage = { sender, content, msg_type: msgType };
    const allLogs: string[] = [];

    try {
      // Step 1: Execute onRequest on backend (sandbox)
      const step1 = await api.debugRequest({ script, webhook_url: webhookUrl, mock_message: mockMessage });
      allLogs.push(...(step1.logs || []));

      if (step1.error || step1.skipped) {
        setResult({ ...step1, logs: allLogs });
        setRunning(false);
        return;
      }

      // Step 2: Frontend sends HTTP request
      let httpResponse: any = null;
      if (step1.request) {
        allLogs.push(`→ 前端发送 ${step1.request.method} ${step1.request.url}`);
        try {
          const res = await fetch(step1.request.url, {
            method: step1.request.method,
            headers: step1.request.headers,
            body: step1.request.body,
          });
          const body = await res.text();
          const headers: Record<string, string> = {};
          res.headers.forEach((v, k) => { headers[k] = v; });
          httpResponse = { status: res.status, headers, body };
          allLogs.push(`✓ 响应 ${res.status} (${body.length} 字节)`);
        } catch (err: any) {
          allLogs.push(`✕ HTTP 请求失败: ${err.message}`);
        }
      }

      // Step 3: Execute onResponse on backend (sandbox)
      let step3: any = null;
      if (httpResponse) {
        step3 = await api.debugResponse({ script, mock_message: mockMessage, response: httpResponse });
        allLogs.push(...(step3.logs || []));
      }

      setResult({
        request: step1.request,
        response: httpResponse,
        replies: [...(step1.replies || []), ...(step3?.replies || [])],
        skipped: false,
        error: step3?.error || "",
        logs: allLogs,
        permissions: step1.permissions,
      });
    } catch (err: any) {
      allLogs.push(`✕ 错误: ${err.message}`);
      setResult({ error: err.message, logs: allLogs });
    }
    setRunning(false);
  }

  function loadPlugin(p: any) {
    api.getPlugin(p.id).then((detail) => {
      if (detail.script) setScript(detail.script);
      setShowPicker(false);
    }).catch(() => setShowPicker(false));
  }

  const allPlugins = [...myPlugins.filter((m) => !plugins.some((p) => p.id === m.id)), ...plugins];

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold">插件调试器</h1>
        <p className="text-xs text-muted-foreground mt-0.5">
          完整执行插件脚本：解析 → onRequest → 实际 HTTP 请求 → onResponse → reply
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Left: Script */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <p className="text-xs font-medium">脚本</p>
            <div className="relative">
              <Button variant="outline" size="sm" className="text-[10px] h-6" onClick={() => setShowPicker(!showPicker)}>
                加载已有插件 <ChevronDown className="w-3 h-3 ml-1" />
              </Button>
              {showPicker && (
                <div className="absolute right-0 top-7 z-10 w-64 border rounded-lg bg-background shadow-lg max-h-56 overflow-y-auto">
                  {allPlugins.length === 0 && <p className="text-[10px] text-muted-foreground p-3 text-center">暂无插件</p>}
                  {allPlugins.map((p) => (
                    <button key={p.id} onClick={() => loadPlugin(p)}
                      className="w-full text-left px-3 py-2 text-xs hover:bg-secondary cursor-pointer border-b last:border-0 flex items-center justify-between">
                      <span>{p.icon} {p.name} <span className="text-muted-foreground">v{p.version}</span></span>
                      <span className="text-[10px] text-muted-foreground">{p.status}</span>
                    </button>
                  ))}
                  <button onClick={() => setShowPicker(false)} className="w-full text-center text-[10px] text-muted-foreground py-1.5 hover:text-primary cursor-pointer">关闭</button>
                </div>
              )}
            </div>
          </div>
          <textarea
            value={script}
            onChange={(e) => setScript(e.target.value)}
            rows={24}
            className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-[11px] font-mono placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:border-ring resize-none"
          />
        </div>

        {/* Right: Config + Result */}
        <div className="space-y-3">
          {/* Config */}
          <Card className="space-y-2 p-3">
            <p className="text-xs font-medium">请求配置</p>
            <div>
              <label className="text-[10px] text-muted-foreground">Webhook URL</label>
              <Input value={webhookUrl} onChange={(e) => setWebhookUrl(e.target.value)} className="h-7 text-[11px] font-mono" />
            </div>
            <p className="text-xs font-medium mt-2">模拟消息</p>
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="text-[10px] text-muted-foreground">发送者</label>
                <Input value={sender} onChange={(e) => setSender(e.target.value)} className="h-7 text-[11px]" />
              </div>
              <div>
                <label className="text-[10px] text-muted-foreground">消息类型</label>
                <select value={msgType} onChange={(e) => setMsgType(e.target.value)}
                  className="w-full h-7 text-[11px] rounded-md border border-input bg-transparent px-2">
                  {["text", "image", "voice", "video", "file"].map((t) => <option key={t} value={t}>{t}</option>)}
                </select>
              </div>
            </div>
            <div>
              <label className="text-[10px] text-muted-foreground">消息内容</label>
              <Input value={content} onChange={(e) => setContent(e.target.value)} className="h-7 text-[11px]" />
            </div>
            <Button size="sm" onClick={handleRun} disabled={running} className="w-full mt-1">
              <Play className="w-3.5 h-3.5 mr-1" /> {running ? "执行中..." : "运行"}
            </Button>
          </Card>

          {/* Result */}
          {result && (
            <div className="space-y-2">
              {/* Logs */}
              <Card className="p-3 space-y-0.5">
                <p className="text-[10px] font-medium mb-1">执行日志</p>
                {(result.logs || []).map((log: string, i: number) => (
                  <p key={i} className={`text-[10px] font-mono ${
                    log.startsWith("✓") ? "text-primary" : log.startsWith("✕") ? "text-destructive" : log.startsWith("⚠") ? "text-yellow-500" : "text-muted-foreground"
                  }`}>{log}</p>
                ))}
                {result.error && <p className="text-[10px] font-mono text-destructive">✕ {result.error}</p>}
              </Card>

              {/* Permissions */}
              {result.permissions && (
                <Card className="p-3">
                  <p className="text-[10px] font-medium mb-1">解析结果</p>
                  <div className="text-[10px] text-muted-foreground space-y-0.5">
                    <p>@grant: {(result.permissions.grants || []).join(", ") || "（未声明，默认全开）"}</p>
                    <p>@match: {result.permissions.match || "*"}</p>
                    <p>@connect: {result.permissions.connect || "*"}</p>
                  </div>
                </Card>
              )}

              {/* Replies */}
              {(result.replies || []).length > 0 && (
                <Card className="p-3">
                  <p className="text-[10px] font-medium mb-1">reply() 输出 ({result.replies.length})</p>
                  {result.replies.map((r: string, i: number) => (
                    <p key={i} className="text-[10px] font-mono text-primary">{r}</p>
                  ))}
                </Card>
              )}

              {/* Request */}
              {result.request && (
                <Card className="overflow-hidden">
                  <div className="px-3 py-1.5 border-b flex items-center justify-between bg-secondary/30">
                    <p className="text-[10px] font-medium">请求</p>
                    <span className="text-[10px] font-mono text-muted-foreground">{result.request.method} {result.request.url}</span>
                  </div>
                  {Object.keys(result.request.headers || {}).length > 0 && (
                    <div className="px-3 py-1.5 border-b text-[10px] text-muted-foreground">
                      {Object.entries(result.request.headers).map(([k, v]) => (
                        <p key={k} className="font-mono">{k}: {v as string}</p>
                      ))}
                    </div>
                  )}
                  <pre className="px-3 py-2 text-[10px] font-mono overflow-x-auto max-h-40 overflow-y-auto whitespace-pre-wrap">
                    {tryPrettyJSON(result.request.body)}
                  </pre>
                </Card>
              )}

              {/* Response */}
              {result.response && (
                <Card className="overflow-hidden">
                  <div className="px-3 py-1.5 border-b flex items-center justify-between bg-secondary/30">
                    <p className="text-[10px] font-medium">响应</p>
                    <span className={`text-[10px] font-mono ${result.response.status < 400 ? "text-primary" : "text-destructive"}`}>
                      {result.response.status}
                    </span>
                  </div>
                  <pre className="px-3 py-2 text-[10px] font-mono overflow-x-auto max-h-48 overflow-y-auto whitespace-pre-wrap">
                    {tryPrettyJSON(result.response.body)}
                  </pre>
                </Card>
              )}

              {result.skipped && (
                <Card className="p-3 border-yellow-500/30 bg-yellow-500/5">
                  <p className="text-[10px] text-yellow-500">⚠ skip() 被调用 — 部署后不会发送 HTTP 请求</p>
                </Card>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function tryPrettyJSON(s: string): string {
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; }
}
