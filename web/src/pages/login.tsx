import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Card } from "../components/ui/card";
import { api } from "../lib/api";

const providerLabels: Record<string, string> = {
  github: "GitHub",
  linuxdo: "LinuxDo",
};

export function LoginPage() {
  const navigate = useNavigate();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [oauthProviders, setOauthProviders] = useState<string[]>([]);

  useEffect(() => {
    api.oauthProviders().then((data) => setOauthProviders(data.providers || [])).catch(() => {});
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      if (mode === "register") {
        await api.register(username, password);
      } else {
        await api.login(username, password);
      }
      navigate("/");
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  function handleOAuth(provider: string) {
    window.location.href = `/api/auth/oauth/${provider}`;
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <Card className="w-full max-w-sm space-y-6">
        <div className="text-center">
          <h1 className="text-xl font-semibold">OpenILink Hub</h1>
          <p className="text-sm text-muted-foreground mt-1">
            {mode === "login" ? "登录你的账号" : "创建新账号"}
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <Input
            placeholder="用户名"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoFocus
          />
          <Input
            type="password"
            placeholder="密码 (至少8位)"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" className="w-full" disabled={loading}>
            {loading ? "..." : mode === "login" ? "登录" : "注册"}
          </Button>
        </form>

        {oauthProviders.length > 0 && (
          <>
            <div className="relative">
              <div className="absolute inset-0 flex items-center">
                <span className="w-full border-t" />
              </div>
              <div className="relative flex justify-center text-xs uppercase">
                <span className="bg-card px-2 text-muted-foreground">或</span>
              </div>
            </div>
            <div className="space-y-2">
              {oauthProviders.map((provider) => (
                <Button
                  key={provider}
                  type="button"
                  variant="outline"
                  className="w-full"
                  onClick={() => handleOAuth(provider)}
                >
                  使用 {providerLabels[provider] || provider} 登录
                </Button>
              ))}
            </div>
          </>
        )}

        <p className="text-center text-sm text-muted-foreground">
          {mode === "login" ? "没有账号？" : "已有账号？"}
          <button
            type="button"
            className="text-primary ml-1 hover:underline cursor-pointer"
            onClick={() => { setMode(mode === "login" ? "register" : "login"); setError(""); }}
          >
            {mode === "login" ? "注册" : "登录"}
          </button>
        </p>
      </Card>
    </div>
  );
}
