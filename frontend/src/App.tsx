import { Button, ConfigProvider, Layout, Menu, Space, Tag, Typography, theme } from "antd";
import { useMemo } from "react";
import { Link, Route, Routes, useLocation, useNavigate } from "react-router-dom";

import "./index.css";

import { AgentPage } from "./pages/AgentPage";
import { CommandsPage } from "./pages/CommandsPage";
import { DashboardPage } from "./pages/DashboardPage";

export function App() {
  const location = useLocation();
  const navigate = useNavigate();
  const menuItems = useMemo(
    () => [
      { key: "/", label: "总览" },
      { key: "/agent", label: "Agent 控制台" },
      { key: "/commands", label: "快捷命令" },
    ],
    [],
  );

  return (
    <ConfigProvider
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: "#0c8a7a",
          colorInfo: "#0c8a7a",
          colorSuccess: "#198754",
          colorWarning: "#c37b1d",
          colorTextBase: "#1d2733",
          borderRadius: 18,
          fontFamily: '"Avenir Next", "PingFang SC", "Hiragino Sans GB", "Noto Sans CJK SC", sans-serif',
        },
      }}
    >
      <Layout className="shell-layout">
        <Layout.Sider breakpoint="lg" collapsedWidth={0} width={260} className="shell-sider">
          <div className="shell-brand">
            <Typography.Text className="shell-eyebrow">Bun + MiniClaw</Typography.Text>
            <Typography.Title level={3}>本地 Agent 控制台</Typography.Title>
            <Typography.Paragraph type="secondary">
              一个前端目录，直接把本地 CLI 能力铺到 Web UI。
            </Typography.Paragraph>
          </div>
          <Menu
            className="shell-menu"
            mode="inline"
            selectedKeys={[location.pathname]}
            items={menuItems}
            onClick={({ key }) => navigate(key)}
          />
        </Layout.Sider>
        <Layout>
          <Layout.Header className="shell-header">
            <Space size="middle" className="header-copy">
              <div>
                <Typography.Text className="shell-eyebrow">Frontend Workspace</Typography.Text>
                <Typography.Title level={4}>MiniClaw Console</Typography.Title>
              </div>
              <Tag color="processing">Local CLI Bridge</Tag>
            </Space>
            <Button type="default">
              <Link to="/agent">执行 Prompt</Link>
            </Button>
          </Layout.Header>
          <Layout.Content className="shell-content">
            <Routes>
              <Route path="/" element={<DashboardPage />} />
              <Route path="/agent" element={<AgentPage />} />
              <Route path="/commands" element={<CommandsPage />} />
            </Routes>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}

export default App;
