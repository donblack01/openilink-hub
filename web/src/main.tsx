import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import "./index.css";
import { HomePage } from "./pages/home";
import { LoginPage } from "./pages/login";
import { Layout } from "./components/layout";
import { BotsPage } from "./pages/bots";
import { BotDetailPage } from "./pages/bot-detail";
import { SettingsPage } from "./pages/settings";
import { PluginsPage } from "./pages/plugins";
import { ChannelDetailPage } from "./pages/channel-detail";
import { AdminPage } from "./pages/admin";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <Routes>
        {/* Public */}
        <Route path="/" element={<HomePage />} />
        <Route path="/webhook-plugins" element={<PluginsPage />} />
        <Route path="/login" element={<LoginPage />} />
        {/* Dashboard (authenticated, sidebar layout) */}
        <Route path="/dashboard" element={<Layout />}>
          <Route index element={<BotsPage />} />
          <Route path="bot/:id" element={<BotDetailPage />} />
          <Route path="bot/:id/channel/:cid" element={<ChannelDetailPage />} />
          <Route path="webhook-plugins" element={<PluginsPage embedded />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="admin" element={<AdminPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  </StrictMode>
);
