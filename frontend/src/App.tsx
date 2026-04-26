import { Button, ConfigProvider, Layout, Tag, Typography, theme } from "antd";
import { useEffect, useMemo, useState } from "react";
import { Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";

import "./index.css";

import { AgentPage, type AgentSectionKey } from "./pages/AgentPage";
import { CommandsPage } from "./pages/CommandsPage";
import { DashboardPage } from "./pages/DashboardPage";
import { emitPromptRequested, historyUpdatedEvent, readPromptHistory, type HistoryUpdatedDetail, type PromptHistoryItem } from "./workbenchEvents";
import { WorkspaceStatusProvider } from "./workspaceStatus";

const defaultAgentSection: AgentSectionKey = "composer";
const validAgentSections = new Set<AgentSectionKey>(["composer", "scenes", "workspace", "history"]);

function resolveAgentSection(hash: string): AgentSectionKey {
  const section = hash.replace(/^#/, "");
  return validAgentSections.has(section as AgentSectionKey) ? (section as AgentSectionKey) : defaultAgentSection;
}

function describeHistorySource(item: PromptHistoryItem) {
	return item.source === "command" ? "快捷命令" : "Agent 任务";
}

export function App() {
  const location = useLocation();
  const navigate = useNavigate();
  const [activeSection, setActiveSection] = useState<AgentSectionKey>(defaultAgentSection);
  const [recentHistory, setRecentHistory] = useState<PromptHistoryItem[]>([]);
  const pageItems = useMemo(
    () => [
      { key: "agent", path: "/agent", label: "任务工作台", helper: "输入、执行和回放任务" },
      { key: "dashboard", path: "/dashboard", label: "运行概览", helper: "查看健康状态和 status 输出" },
      { key: "commands", path: "/commands", label: "快捷命令", helper: "直接执行常用命令" },
    ],
    [],
  );
  const workflowItems = useMemo(
    () => [
      { key: "composer", label: "新建任务", helper: "直接开始一个任务" },
      { key: "scenes", label: "任务场景", helper: "从模板和专家起步" },
      { key: "workspace", label: "工作区", helper: "查看状态和配置" },
    ],
    [],
  );
  const exploreItems = useMemo(
    () => [
      { key: "history", label: "运行记录", helper: "回看最近执行" },
    ],
    [],
  );
  const currentPageKey = location.pathname === "/dashboard" ? "dashboard" : location.pathname === "/commands" ? "commands" : "agent";
  const isAgentPage = currentPageKey === "agent";
  const requestedSection = isAgentPage ? resolveAgentSection(location.hash) : defaultAgentSection;
  const pageMeta = useMemo(() => {
    switch (currentPageKey) {
      case "dashboard":
        return { eyebrow: "运行概览", tag: "CLI Health" };
      case "commands":
        return { eyebrow: "快捷命令", tag: "Command Presets" };
      default:
        return { eyebrow: "任务工作台", tag: "Task Runner" };
    }
  }, [currentPageKey]);

  useEffect(() => {
    setRecentHistory(readPromptHistory().slice(0, 8));

    function handleHistoryUpdated(event: Event) {
      const detail = (event as CustomEvent<HistoryUpdatedDetail>).detail;
      setRecentHistory((detail?.history ?? []).slice(0, 8));
    }

    window.addEventListener(historyUpdatedEvent, handleHistoryUpdated as EventListener);
    return () => {
      window.removeEventListener(historyUpdatedEvent, handleHistoryUpdated as EventListener);
    };
  }, []);

  useEffect(() => {
    if (!isAgentPage) {
      return;
    }
    setActiveSection(requestedSection);
  }, [isAgentPage, requestedSection]);

  function openPage(path: string) {
    navigate(path);
  }

  function openAgentSection(sectionId: AgentSectionKey) {
    setActiveSection(sectionId);
    if (location.pathname === "/agent" && location.hash === `#${sectionId}`) {
      document.getElementById(sectionId)?.scrollIntoView({ behavior: "smooth", block: "start" });
      return;
    }
    navigate(`/agent#${sectionId}`);
  }

  function useHistoryPrompt(prompt: string) {
    emitPromptRequested(prompt);
    openAgentSection("composer");
  }

  return (
    <ConfigProvider
      componentSize="small"
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: "#0c8a7a",
          colorInfo: "#0c8a7a",
          colorSuccess: "#198754",
          colorWarning: "#c37b1d",
          colorTextBase: "#1d2733",
          borderRadius: 16,
          fontFamily: '"Avenir Next", "PingFang SC", "Hiragino Sans GB", "Noto Sans CJK SC", sans-serif',
        },
      }}
    >
      <WorkspaceStatusProvider>
        <Layout className="shell-layout">
          <Layout.Sider breakpoint="lg" collapsedWidth={0} width={248} className="shell-sider">
            <div className="shell-brand">
              <Typography.Text className="shell-eyebrow">MiniClaw Workspace</Typography.Text>
              <Typography.Text className="shell-brand-note">任务 · 状态 · 运行记录</Typography.Text>
            </div>
            <div className="shell-sidebar-body">
              <section className="shell-nav-group">
                <Typography.Text className="shell-nav-label">页面</Typography.Text>
                <div className="shell-nav-stack">
                  {pageItems.map(item => (
                    <button
                      key={item.key}
                      type="button"
                      className={`shell-nav-button${currentPageKey === item.key ? " is-active" : ""}`}
                      onClick={() => openPage(item.path)}
                    >
                      <span className="shell-nav-title">{item.label}</span>
                      <span className="shell-nav-helper">{item.helper}</span>
                    </button>
                  ))}
                </div>
              </section>

              {isAgentPage ? (
                <>
                  <section className="shell-nav-group">
                    <Typography.Text className="shell-nav-label">工作流</Typography.Text>
                    <div className="shell-nav-stack">
                      {workflowItems.map(item => (
                        <button
                          key={item.key}
                          type="button"
                          className={`shell-nav-button${activeSection === item.key ? " is-active" : ""}`}
                          onClick={() => openAgentSection(item.key as AgentSectionKey)}
                        >
                          <span className="shell-nav-title">{item.label}</span>
                          <span className="shell-nav-helper">{item.helper}</span>
                        </button>
                      ))}
                    </div>
                  </section>

                  <section className="shell-nav-group">
                    <Typography.Text className="shell-nav-label">探索入口</Typography.Text>
                    <div className="shell-nav-stack shell-nav-stack-compact">
                      {exploreItems.map(item => (
                        <button
                          key={item.label}
                          type="button"
                          className={`shell-nav-button shell-nav-button-secondary${activeSection === item.key ? " is-active" : ""}`}
                          onClick={() => openAgentSection(item.key as AgentSectionKey)}
                        >
                          <span className="shell-nav-title">{item.label}</span>
                          <span className="shell-nav-helper">{item.helper}</span>
                        </button>
                      ))}
                    </div>
                  </section>
                </>
              ) : (
                <section className="shell-nav-group">
                  <Typography.Text className="shell-nav-label">快速入口</Typography.Text>
                  <div className="shell-nav-stack shell-nav-stack-compact">
                    <button type="button" className="shell-nav-button shell-nav-button-secondary" onClick={() => openAgentSection("composer")}>
                      <span className="shell-nav-title">开始任务</span>
                      <span className="shell-nav-helper">返回工作台直接输入任务</span>
                    </button>
                    <button type="button" className="shell-nav-button shell-nav-button-secondary" onClick={() => openAgentSection("history")}>
                      <span className="shell-nav-title">查看运行记录</span>
                      <span className="shell-nav-helper">回到工作台查看最近执行</span>
                    </button>
                  </div>
                </section>
              )}

              <section className="shell-nav-group shell-history-group">
                <div className="shell-history-head">
                  <Typography.Text className="shell-nav-label">最近任务</Typography.Text>
                  <Button type="link" size="small" onClick={() => openAgentSection("history")}>查看全部</Button>
                </div>
                {recentHistory.length === 0 ? (
                  <div className="shell-history-empty">
                    <Typography.Text type="secondary">执行一次任务后，这里会出现最近记录。</Typography.Text>
                  </div>
                ) : (
                  <div className="shell-history-list">
                    {recentHistory.map(item => (
                      <button key={`${item.runAt}-${item.prompt}`} type="button" className="shell-history-item" onClick={() => useHistoryPrompt(item.prompt)}>
                        <span className="shell-history-status" data-state={item.exitCode === 0 ? "success" : "error"} />
                        <span className="shell-history-copy">
                          <span className="shell-history-title">{item.title}</span>
                          <span className="shell-history-meta">{describeHistorySource(item)} · {new Date(item.runAt).toLocaleString()}</span>
                        </span>
                      </button>
                    ))}
                  </div>
                )}
              </section>
            </div>
          </Layout.Sider>
          <Layout className="shell-main">
            <Layout.Header className="shell-header">
              <div className="header-copy">
                <Typography.Text className="shell-header-label">{pageMeta.eyebrow}</Typography.Text>
                <Tag color="processing">{pageMeta.tag}</Tag>
              </div>
              <Button type="default" onClick={() => openAgentSection("composer")}>
                开始任务
              </Button>
            </Layout.Header>
            <Layout.Content className="shell-content">
              <Routes>
                <Route path="/" element={<Navigate to="/agent" replace />} />
                <Route path="/agent" element={<AgentPage requestedSection={requestedSection} onSectionActive={setActiveSection} />} />
                <Route path="/dashboard" element={<DashboardPage />} />
                <Route path="/commands" element={<CommandsPage />} />
                <Route path="*" element={<Navigate to="/agent" replace />} />
              </Routes>
            </Layout.Content>
          </Layout>
        </Layout>
      </WorkspaceStatusProvider>
    </ConfigProvider>
  );
}

export default App;
