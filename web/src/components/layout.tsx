import { Outlet, useNavigate, Link } from "react-router-dom";
import { useEffect, useState } from "react";
import { LogOut, Settings, Github } from "lucide-react";
import { api } from "../lib/api";

export function Layout() {
  const navigate = useNavigate();
  const [user, setUser] = useState<any>(null);

  useEffect(() => {
    api.me().then(setUser).catch(() => navigate("/login"));
  }, []);

  if (!user) return null;

  async function handleLogout() {
    await api.logout();
    navigate("/login");
  }

  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b px-6 py-3 flex items-center justify-between shrink-0">
        <div className="flex items-center gap-2">
          <Link to="/" className="font-semibold text-sm hover:opacity-80">OpenILink Hub</Link>
          <a href="https://github.com/openilink/openilink-hub" target="_blank" rel="noopener" className="text-muted-foreground hover:text-foreground">
            <Github className="w-4 h-4" />
          </a>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-muted-foreground">{user.username}</span>
          <Link to="/settings" className="text-muted-foreground hover:text-foreground">
            <Settings className="w-4 h-4" />
          </Link>
          <button onClick={handleLogout} className="text-muted-foreground hover:text-foreground cursor-pointer">
            <LogOut className="w-4 h-4" />
          </button>
        </div>
      </header>
      <main className="flex-1 p-6 overflow-auto max-w-4xl mx-auto w-full">
        <Outlet />
      </main>
    </div>
  );
}
