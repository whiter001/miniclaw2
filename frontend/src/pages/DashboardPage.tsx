import { Alert, Card, Col, Row, Space, Statistic, Tag, Typography } from "antd";
import { useEffect, useState } from "react";

import { fetchHealth, runCli, type CliRunResponse, type HealthResponse } from "../api";
import { RunResultPanel } from "../components/RunResultPanel";

export function DashboardPage() {
	const [health, setHealth] = useState<HealthResponse | null>(null);
	const [statusResult, setStatusResult] = useState<CliRunResponse | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string>();

	useEffect(() => {
		let active = true;

		async function load() {
			setLoading(true);
			setError(undefined);
			try {
				const [healthPayload, statusPayload] = await Promise.all([fetchHealth(), runCli({ command: "status" })]);
				if (!active) {
					return;
				}
				setHealth(healthPayload);
				setStatusResult(statusPayload);
			} catch (loadError) {
				if (!active) {
					return;
				}
				setError(loadError instanceof Error ? loadError.message : String(loadError));
			} finally {
				if (active) {
					setLoading(false);
				}
			}
		}

		void load();
		return () => {
			active = false;
		};
	}, []);

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