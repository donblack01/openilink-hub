import { useEffect, useState } from "react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Card } from "../components/ui/card";
import { api } from "../lib/api";
import { Save, Trash2 } from "lucide-react";

const providerLabels: Record<string, string> = { github: "GitHub", linuxdo: "LinuxDo" };
const providerCallbackHelp: Record<string, string> = {
  github: "在 github.com/settings/developers → OAuth Apps 中创建应用",
  linuxdo: "在 connect.linux.do 中创建应用",
};

export function AdminPage() {
  const [tab, setTab] = useState<"dashboard" | "users" | "system" | "ai" | "oauth">("dashboard");

  const tabs = [
    { key: "dashboard", label: "概览" },
    { key: "users", label: "用户管理" },
    { key: "system", label: "系统状态" },
    { key: "ai", label: "AI 配置" },
    { key: "oauth", label: "OAuth 配置" },
  ] as const;

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold">系统管理</h1>
        <p className="text-xs text-muted-foreground mt-0.5">管理用户、系统配置、AI 和 OAuth</p>
      </div>

      <div className="flex border rounded-lg overflow-hidden w-fit">
        {tabs.map((t) => (
          <button key={t.key} className={`px-3 py-1.5 text-xs cursor-pointer ${tab === t.key ? "bg-secondary font-medium" : "text-muted-foreground"}`}
            onClick={() => setTab(t.key)}>{t.label}</button>
        ))}
      </div>

      {tab === "dashboard" && <DashboardTab />}
      {tab === "users" && <UsersTab />}
      {tab === "system" && <SystemTab />}
      {tab === "ai" && <AITab />}
      {tab === "oauth" && <OAuthTab />}
    </div>
  );
}

// ==================== Dashboard ====================

function DashboardTab() {
  const [stats, setStats] = useState<any>(null);
  useEffect(() => {
    api.adminStats().then(setStats).catch(() => {});
    const t = setInterval(() => api.adminStats().then(setStats).catch(() => {}), 10000);
    return () => clearInterval(t);
  }, []);
  if (!stats) return null;

  const items = [
    { label: "用户", value: stats.total_users, sub: `${stats.active_users} 活跃` },
    { label: "Bot", value: stats.total_bots, sub: `${stats.online_bots} 在线${stats.expired_bots > 0 ? ` / ${stats.expired_bots} 过期` : ""}` },
    { label: "渠道", value: stats.total_channels },
    { label: "WebSocket", value: stats.connected_ws, sub: "在线连接" },
    { label: "总消息", value: stats.total_messages.toLocaleString(), sub: `${stats.inbound_messages.toLocaleString()} 入 / ${stats.outbound_messages.toLocaleString()} 出` },
  ];

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
      {items.map((item) => (
        <div key={item.label} className="p-4 rounded-lg border bg-card text-center">
          <p className="text-2xl font-bold">{item.value}</p>
          <p className="text-xs text-muted-foreground">{item.label}</p>
          {item.sub && <p className="text-[10px] text-muted-foreground mt-0.5">{item.sub}</p>}
        </div>
      ))}
    </div>
  );
}

// ==================== Users ====================

function UsersTab() {
  const [users, setUsers] = useState<any[]>([]);
  const [showCreate, setShowCreate] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newRole, setNewRole] = useState("member");
  const [error, setError] = useState("");
  const [resetTarget, setResetTarget] = useState<string | null>(null);
  const [resetPwd, setResetPwd] = useState("");

  async function load() { try { setUsers(await api.listUsers() || []); } catch {} }
  useEffect(() => { load(); }, []);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault(); setError("");
    if (!newUsername.trim() || newPassword.length < 8) { setError("用户名必填，密码至少 8 位"); return; }
    try {
      await api.createUser({ username: newUsername.trim(), password: newPassword, role: newRole });
      setNewUsername(""); setNewPassword(""); setShowCreate(false); load();
    } catch (err: any) { setError(err.message); }
  }

  async function handleToggleRole(user: any) {
    const r = user.role === "admin" ? "member" : "admin";
    if (!confirm(`将 ${user.username} 改为 ${r === "admin" ? "管理员" : "成员"}？`)) return;
    try { await api.updateUserRole(user.id, r); load(); } catch (err: any) { setError(err.message); }
  }

  async function handleToggleStatus(user: any) {
    const s = user.status === "active" ? "disabled" : "active";
    if (!confirm(`${s === "disabled" ? "禁用" : "启用"} ${user.username}？`)) return;
    try { await api.updateUserStatus(user.id, s); load(); } catch (err: any) { setError(err.message); }
  }

  async function handleResetPassword() {
    if (!resetTarget || resetPwd.length < 8) { setError("密码至少 8 位"); return; }
    try { await api.resetUserPassword(resetTarget, resetPwd); setResetTarget(null); setResetPwd(""); setError(""); }
    catch (err: any) { setError(err.message); }
  }

  async function handleDelete(user: any) {
    if (!confirm(`永久删除 ${user.username}？不可撤销。`)) return;
    try { await api.deleteUser(user.id); load(); } catch (err: any) { setError(err.message); }
  }

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <Button variant="outline" size="sm" className="text-xs h-7" onClick={() => setShowCreate(!showCreate)}>
          {showCreate ? "取消" : "创建用户"}
        </Button>
      </div>
      {error && <p className="text-[10px] text-destructive">{error}</p>}

      {showCreate && (
        <form onSubmit={handleCreate} className="p-3 rounded-lg border bg-card space-y-2">
          <div className="flex gap-2">
            <Input placeholder="用户名" value={newUsername} onChange={(e) => setNewUsername(e.target.value)} className="h-7 text-xs" />
            <Input type="password" placeholder="密码（至少 8 位）" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} className="h-7 text-xs" />
          </div>
          <div className="flex items-center justify-between">
            <div className="flex gap-1">
              {["member", "admin"].map((r) => (
                <button key={r} type="button" onClick={() => setNewRole(r)}
                  className={`px-2 py-0.5 text-[10px] rounded cursor-pointer ${newRole === r ? "bg-primary text-primary-foreground" : "bg-secondary"}`}>
                  {r === "admin" ? "管理员" : "成员"}
                </button>
              ))}
            </div>
            <Button type="submit" size="sm" className="h-7 text-xs">创建</Button>
          </div>
        </form>
      )}

      <div className="space-y-1">
        {users.map((u) => (
          <div key={u.id} className="flex items-center justify-between p-2.5 rounded-lg border bg-card">
            <div className="flex items-center gap-2">
              <div className="w-7 h-7 rounded-full bg-secondary flex items-center justify-center text-[10px] font-medium">
                {u.username.charAt(0).toUpperCase()}
              </div>
              <div>
                <div className="flex items-center gap-1.5">
                  <span className="text-xs font-medium">{u.username}</span>
                  <span className={`text-[10px] px-1 rounded ${u.role === "admin" ? "bg-primary/10 text-primary" : "bg-secondary text-muted-foreground"}`}>
                    {u.role === "admin" ? "管理员" : "成员"}
                  </span>
                  {u.status === "disabled" && <span className="text-[10px] px-1 rounded bg-destructive/10 text-destructive">已禁用</span>}
                </div>
                {u.email && <p className="text-[10px] text-muted-foreground">{u.email}</p>}
              </div>
            </div>
            <div className="flex items-center gap-1">
              <button onClick={() => handleToggleRole(u)} className="text-[10px] text-muted-foreground hover:text-foreground px-1.5 py-0.5 rounded hover:bg-secondary cursor-pointer">{u.role === "admin" ? "降级" : "升级"}</button>
              <button onClick={() => handleToggleStatus(u)} className="text-[10px] text-muted-foreground hover:text-foreground px-1.5 py-0.5 rounded hover:bg-secondary cursor-pointer">{u.status === "active" ? "禁用" : "启用"}</button>
              <button onClick={() => { setResetTarget(u.id); setResetPwd(""); }} className="text-[10px] text-muted-foreground hover:text-foreground px-1.5 py-0.5 rounded hover:bg-secondary cursor-pointer">重置密码</button>
              <button onClick={() => handleDelete(u)} className="text-[10px] text-destructive px-1.5 py-0.5 rounded hover:bg-destructive/10 cursor-pointer">删除</button>
            </div>
          </div>
        ))}
      </div>

      {resetTarget && (
        <div className="p-3 rounded-lg border bg-card space-y-2">
          <p className="text-xs font-medium">重置密码 — {users.find((u) => u.id === resetTarget)?.username}</p>
          <div className="flex gap-2">
            <Input type="password" placeholder="新密码（至少 8 位）" value={resetPwd} onChange={(e) => setResetPwd(e.target.value)} className="h-7 text-xs" autoFocus />
            <Button size="sm" className="h-7 text-xs" onClick={handleResetPassword}>确认</Button>
            <Button size="sm" variant="ghost" className="h-7 text-xs" onClick={() => setResetTarget(null)}>取消</Button>
          </div>
        </div>
      )}
    </div>
  );
}

// ==================== System ====================

function SystemTab() {
  const [info, setInfo] = useState<any>(null);
  useEffect(() => { api.info().then(setInfo).catch(() => {}); }, []);
  if (!info) return null;

  return (
    <div className="space-y-1.5">
      {[
        { label: "AI 服务", enabled: info.ai },
        { label: "对象存储 (MinIO)", enabled: info.storage },
      ].map((item) => (
        <div key={item.label} className="flex items-center justify-between text-sm p-3 rounded-lg border bg-card">
          <span>{item.label}</span>
          <span className={`text-xs px-2 py-0.5 rounded ${item.enabled ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground"}`}>
            {item.enabled ? "已启用" : "未配置"}
          </span>
        </div>
      ))}
    </div>
  );
}

// ==================== AI ====================

function AITab() {
  const [config, setConfig] = useState<any>(null);
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [model, setModel] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [maxHistory, setMaxHistory] = useState(20);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    try {
      const d = await api.getAIConfig(); setConfig(d);
      setBaseUrl(d.base_url || ""); setModel(d.model || "");
      setSystemPrompt(d.system_prompt || ""); setMaxHistory(parseInt(d.max_history) || 20); setApiKey("");
    } catch {}
  }
  useEffect(() => { load(); }, []);
  if (!config) return null;
  const configured = config.enabled === "true";

  async function handleSave() {
    setSaving(true); setError("");
    try {
      let url = baseUrl.replace(/\/+$/, "");
      if (url && !url.endsWith("/v1")) url += "/v1";
      setBaseUrl(url);
      await api.setAIConfig({ base_url: url, api_key: apiKey || undefined, model: model || undefined, system_prompt: systemPrompt, max_history: String(maxHistory || 20) });
      load();
    } catch (err: any) { setError(err.message); }
    setSaving(false);
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-xs text-muted-foreground">配置后渠道可选择「内置」模式，无需单独填写 API Key</p>
        {configured && <Button variant="ghost" size="sm" onClick={async () => { if (confirm("删除全局 AI 配置？")) { await api.deleteAIConfig(); load(); } }}><Trash2 className="w-3.5 h-3.5 text-destructive" /></Button>}
      </div>
      <Input placeholder="https://api.openai.com/v1" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} className="h-8 text-xs font-mono" />
      <div className="flex gap-2">
        <Input type="password" placeholder={configured ? `已配置 (${config.api_key})，留空不变` : "API Key"} value={apiKey} onChange={(e) => setApiKey(e.target.value)} className="h-8 text-xs font-mono" />
        <Input placeholder="模型名称" value={model} onChange={(e) => setModel(e.target.value)} className="h-8 text-xs font-mono w-40" />
      </div>
      <textarea placeholder="默认 System Prompt" value={systemPrompt} onChange={(e) => setSystemPrompt(e.target.value)} rows={3}
        className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-xs placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:border-ring resize-none" />
      <div className="flex items-center gap-2">
        <label className="text-[10px] text-muted-foreground">上下文消息数</label>
        <Input type="number" value={maxHistory} onChange={(e) => setMaxHistory(parseInt(e.target.value) || 20)} className="h-8 text-xs w-20" min={1} max={100} />
      </div>
      {error && <p className="text-[10px] text-destructive">{error}</p>}
      <div className="flex justify-end"><Button size="sm" onClick={handleSave} disabled={saving}>保存</Button></div>
    </div>
  );
}

// ==================== OAuth ====================

function OAuthTab() {
  const [config, setConfig] = useState<Record<string, any> | null>(null);
  const [error, setError] = useState("");
  async function loadConfig() { try { setConfig(await api.getOAuthConfig()); } catch {} }
  useEffect(() => { loadConfig(); }, []);
  if (!config) return null;
  const callbackBase = window.location.origin + "/api/auth/oauth/";

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">DB 配置优先于环境变量，无需重启服务。</p>
      {error && <p className="text-xs text-destructive">{error}</p>}
      {Object.keys(providerLabels).map((name) => (
        <OAuthProviderForm key={name} name={name} label={providerLabels[name]} config={config[name]}
          callbackURL={callbackBase + name + "/callback"} help={providerCallbackHelp[name]}
          onSaved={loadConfig} onError={setError} />
      ))}
    </div>
  );
}

function OAuthProviderForm({ name, label, config, callbackURL, help, onSaved, onError }: {
  name: string; label: string; config: any; callbackURL: string; help: string; onSaved: () => void; onError: (msg: string) => void;
}) {
  const [clientId, setClientId] = useState(config?.client_id || "");
  const [clientSecret, setClientSecret] = useState("");
  const [saving, setSaving] = useState(false);
  useEffect(() => { setClientId(config?.client_id || ""); setClientSecret(""); }, [config]);

  async function handleSave() {
    if (!clientId.trim()) { onError("Client ID 不能为空"); return; }
    setSaving(true); onError("");
    try { await api.setOAuthConfig(name, { client_id: clientId.trim(), client_secret: clientSecret }); onSaved(); }
    catch (err: any) { onError(err.message); }
    setSaving(false);
  }

  const source = config?.source;
  const enabled = config?.enabled;

  return (
    <div className="space-y-2 p-3 rounded-lg border bg-card">
      <div className="flex items-center justify-between">
        <div>
          <span className="text-sm font-medium">{label}</span>
          {enabled && <span className={`ml-2 text-[10px] px-1.5 py-0.5 rounded ${source === "db" ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground"}`}>{source === "db" ? "数据库" : "环境变量"}</span>}
        </div>
        {source === "db" && (
          <Button variant="ghost" size="sm" onClick={async () => { if (confirm(`删除 ${label} OAuth？`)) { onError(""); try { await api.deleteOAuthConfig(name); onSaved(); } catch (e: any) { onError(e.message); } } }}>
            <Trash2 className="w-3.5 h-3.5 text-destructive" />
          </Button>
        )}
      </div>
      <Input placeholder="Client ID" value={clientId} onChange={(e) => setClientId(e.target.value)} className="h-8 text-xs font-mono" />
      <Input type="password" placeholder={enabled ? "Client Secret（留空不变）" : "Client Secret"} value={clientSecret} onChange={(e) => setClientSecret(e.target.value)} className="h-8 text-xs font-mono" />
      <div className="flex items-center justify-between">
        <div className="text-[10px] text-muted-foreground space-y-0.5">
          <p>回调：<code className="select-all">{callbackURL}</code></p>
          <p>{help}</p>
        </div>
        <Button size="sm" onClick={handleSave} disabled={saving}><Save className="w-3.5 h-3.5 mr-1" /> 保存</Button>
      </div>
    </div>
  );
}
