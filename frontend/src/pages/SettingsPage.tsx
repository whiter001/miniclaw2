import { Alert, Button, Card, Descriptions, Typography, message } from "antd";
import { useEffect, useState } from "react";

import { fetchHealth, runCli, type CliRunResponse, type HealthResponse } from "../api";

export function SettingsPage() {
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [statusResult, setStatusResult] = useState<CliRunResponse | null>(null);
  const [loading, setLoading] = useState(true);

  async function load() {
    setLoading(true);
    try {
      const [healthPayload, statusPayload] = await Promise.all([fetchHealth(), runCli({ command: "status" })]);
      setHealth(healthPayload);
      setStatusResult(statusPayload);
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <div className="page-section">
      <div className="page-actions">
        <div>
          <Typography.Title level={4} className="page-title">
            设置
          </Typography.Title>
          <Typography.Text type="secondary" className="page-subtitle">
            展示当前 Bun 前端服务、MiniClaw 工作区、模型配置和渠道相关环境信息。
          </Typography.Text>
        </div>
        <Button onClick={() => void load()}>刷新</Button>
      </div>

      <Alert type="info" showIcon message="设置页当前以只读展示为主，便于先核对工作区路径、LLM 配置和网关通道是否符合预期。" />

      <Card loading={loading} size="small" variant="outlined">
        <Descriptions column={1} bordered size="small">
          <Descriptions.Item label="服务名称">{health?.service ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="服务端口">{health?.port ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="仓库根目录">{health?.repoRoot ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="前端目录">{health?.frontendRoot ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="工作区目录">{health?.workspaceDir ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="配置文件">{health?.configPath ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="MCP 配置">{health?.mcpConfigPath ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="CLI 入口">{health?.cliPath ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="运行模式">{health?.binaryMode ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="网关通道">{health?.gatewayChannel ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="模型地址">{health?.llmBaseUrl ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="模型名称">{health?.llmModel ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="已配置 API Key">{health?.hasLlmApiKey ? "是" : "否"}</Descriptions.Item>
          <Descriptions.Item label="启用 MCP">{health?.enableMcp ? "是" : "否"}</Descriptions.Item>
          <Descriptions.Item label="启用 Auto Skills">{health?.enableAutoSkills ? "是" : "否"}</Descriptions.Item>
          <Descriptions.Item label="QQ Webhook">{health?.qqWebhook ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="微信已配置">{health?.weixinConfigured ? "是" : "否"}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="推荐命令" size="small" variant="outlined">
        <div className="command-list">
          <Typography.Text code>cd frontend && bun --hot src/index.ts</Typography.Text>
          <Typography.Text code>./build.sh</Typography.Text>
          <Typography.Text code>./miniclaw status</Typography.Text>
          <Typography.Text code>./miniclaw cron list</Typography.Text>
        </div>
      </Card>

      <Card title="miniclaw status" size="small" variant="outlined">
        <pre className="code-block">{statusResult?.stdout || statusResult?.stderr || "暂无输出"}</pre>
      </Card>
    </div>
  );
}
