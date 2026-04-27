import { Button, Layout, Menu, Space, Typography } from "antd";
import { useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";

const { Content, Header, Sider } = Layout;

const menuItems = [
  { key: "/dashboard", label: "仪表盘" },
  { key: "/tasks", label: "任务管理" },
  { key: "/schedules", label: "定时任务" },
  { key: "/runs", label: "执行记录" },
  { key: "/skills", label: "Skills" },
  { key: "/settings", label: "设置" },
];

export function AppLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();
  const selectedKey = location.pathname;

  return (
    <Layout className="app-shell">
      <Sider width={220} theme="light" collapsible collapsed={collapsed} collapsedWidth={72} trigger={null}>
        <div className={`app-brand${collapsed ? " is-collapsed" : ""}`}>
          <div className="app-brand-mark">MC</div>
          {!collapsed ? (
            <div>
              <Typography.Title level={5} style={{ margin: 0 }}>
                miniclaw
              </Typography.Title>
              <Typography.Text type="secondary">本地工作台</Typography.Text>
            </div>
          ) : null}
        </div>
        <Menu mode="inline" selectedKeys={[selectedKey]} items={menuItems} onClick={({ key }) => navigate(String(key))} />
      </Sider>
      <Layout>
        <Header className="app-header">
          <Space size={10} align="center">
            <Button size="small" onClick={() => setCollapsed(value => !value)}>
              {collapsed ? "展开" : "收起"}
            </Button>
            <Typography.Title level={5} style={{ margin: 0 }}>
              MiniClaw 控制台
            </Typography.Title>
          </Space>
          <Typography.Text type="secondary">任务定义、定时调度、运行记录和 Skills 一处查看</Typography.Text>
        </Header>
        <Content className="app-content">
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
