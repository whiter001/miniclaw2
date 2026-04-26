import { Alert, Button, Card, Col, Row, Space, Statistic, Tag, Typography } from "antd";
import { useNavigate } from "react-router-dom";

import { RunResultPanel } from "../components/RunResultPanel";
import { emitPromptRequested } from "../workbenchEvents";
import { useWorkspaceStatus } from "../workspaceStatus";

export function DashboardPage() {
	const navigate = useNavigate();
	const { health, statusResult, loading, error, refreshWorkspace } = useWorkspaceStatus();

	function openAgentPrompt(prompt: string, section = "composer") {
		emitPromptRequested(prompt);
		navigate(`/agent#${section}`);
	}

	return (
		<div className="page-stack">
			<Card className="hero-card" bordered={false}>
				<Space direction="vertical" size="large" className="hero-copy">
					<div>
						<Tag color="processing">MiniClaw Web Console</Tag>
						<Typography.Title level={1}>在浏览器里直接调起本地 MiniClaw</Typography.Title>
						<Typography.Paragraph className="hero-paragraph">
							这个前端目录使用 Bun full-stack 运行，页面负责输入与展示，服务端负责执行本地 CLI，当前已接通
							 <Typography.Text code>./miniclaw agent -p "..."</Typography.Text>
							 和 <Typography.Text code>./miniclaw status</Typography.Text>。
						</Typography.Paragraph>
					</div>
					<div className="hero-chip-row">
						<span className="brand-chip">Ant Design</span>
						<span className="brand-chip">React Router</span>
						<span className="brand-chip">Bun Server</span>
					</div>
					<Space wrap size="small">
						<Button type="primary" onClick={() => openAgentPrompt("总结当前运行时状态，并指出异常项和下一步建议")}>
							生成诊断任务
						</Button>
						<Button onClick={() => openAgentPrompt("基于当前工作区状态，给出下一步执行清单")}>生成下一步建议</Button>
						<Button onClick={() => navigate("/agent#history")}>查看运行记录</Button>
						<Button onClick={() => void refreshWorkspace()} loading={loading}>
							刷新状态
						</Button>
					</Space>
				</Space>
			</Card>

			{error ? <Alert type="error" showIcon message="初始化失败" description={error} /> : null}

			<Row gutter={[16, 16]}>
				<Col xs={24} md={8}>
					<Card className="metric-card" bordered={false}>
						<Statistic title="CLI 模式" value={health?.binaryMode === "binary" ? "已构建二进制" : "go run 回退"} loading={loading} />
					</Card>
				</Col>
				<Col xs={24} md={8}>
					<Card className="metric-card" bordered={false}>
						<Statistic title="服务时间" value={health?.serverTime ? new Date(health.serverTime).toLocaleTimeString() : "--"} loading={loading} />
					</Card>
				</Col>
				<Col xs={24} md={8}>
					<Card className="metric-card" bordered={false}>
						<Statistic title="Repo Root" value={health?.repoRoot || "--"} loading={loading} />
					</Card>
				</Col>
			</Row>

			<Row gutter={[16, 16]}>
				<Col xs={24} xl={10}>
					<Card title="当前能力" className="panel-card">
						<div className="summary-list">
							<div>
								<Typography.Text strong>执行 Agent Prompt</Typography.Text>
								<Typography.Paragraph type="secondary">
									通过 POST API 调用本地 MiniClaw CLI，回传 stdout、stderr、耗时和退出码。
								</Typography.Paragraph>
							</div>
							<div>
								<Typography.Text strong>快速查看状态</Typography.Text>
								<Typography.Paragraph type="secondary">
									首页会自动执行一次 <Typography.Text code>miniclaw status</Typography.Text>，方便确认运行时配置。
								</Typography.Paragraph>
							</div>
							<div>
								<Typography.Text strong>从状态直接发起任务</Typography.Text>
								<Typography.Paragraph type="secondary">
									概览页可以直接把当前状态转成诊断任务或下一步建议，无需手动复制上下文。
								</Typography.Paragraph>
							</div>
							<div>
								<Typography.Text strong>保留安全边界</Typography.Text>
								<Typography.Paragraph type="secondary">
									API 只开放受控命令，不接受任意 shell 执行，避免前端直接暴露系统命令入口。
								</Typography.Paragraph>
							</div>
						</div>
					</Card>
				</Col>
				<Col xs={24} xl={14}>
					<RunResultPanel
						title="miniclaw status"
						loading={loading}
						error={error}
						result={statusResult}
						emptyText="首页会自动请求一次 miniclaw status。"
					/>
				</Col>
			</Row>
		</div>
	);
}